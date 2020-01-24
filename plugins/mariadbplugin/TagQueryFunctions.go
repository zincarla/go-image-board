package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//GetUserFilterTags returns a slice of tags based on a user's custom filter
func (DBConnection *MariaDBPlugin) GetUserFilterTags(UserID uint64, CollectionContext bool) ([]interfaces.TagInformation, error) {
	var userFilter string
	err := DBConnection.DBHandle.QueryRow("SELECT SearchFilter FROM Users WHERE ID = ?", UserID).Scan(&userFilter)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "GetUserQueryTags", "*", "ERROR", []string{"Failed to get user filter", err.Error()})
		return nil, err
	}
	tags, err := DBConnection.GetQueryTags(userFilter, CollectionContext)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "GetUserQueryTags", "*", "ERROR", []string{"Failed to get tags from user filter", err.Error()})
		return nil, err
	}
	//Loop through the tags and ensure we have them set as FromUserFilter
	for i := 0; i < len(tags); i++ {
		tags[i].FromUserFilter = true
	}
	return tags, nil
}

//GetQueryTags returns a slice of tags based on a query string, CollectionContext should be true if these tags are being parsed for a collection
func (DBConnection *MariaDBPlugin) GetQueryTags(UserQuery string, CollectionContext bool) ([]interfaces.TagInformation, error) {
	//What we want to return
	var ToReturn []interfaces.TagInformation
	//If the user query is blank, just short circuit outta here
	if len(UserQuery) == 0 {
		return ToReturn, nil
	}
	//This splits up the user query into each individual tag name from "-Jaws Movie Best" to "-Jaws", "Movie", "Best"
	RawQueryTags := strings.Fields(UserQuery)
	var ParsedQueryTags []string
	//Join tags that are in quotes
	//The goal here it to take something like
	//"i wrote you a song" audio
	//and turn it into two tags
	//i_wrote_you_a_song, audio
	InQuote := false
	TagConstruct := ""
	var Negate = false //User is specifically negating this tag
	for _, Tag := range RawQueryTags {

		if InQuote == false && Tag[0:1] == "-" {
			Negate = true
			Tag = Tag[1:] //Remove the minus
		}
		if InQuote {
			//TagConsturct should already have something at this point, so add a underscore between it and the new field
			TagConstruct = TagConstruct + "_" + Tag
			//If we now end in a quote, then we add the tag construct as one tag
			if TagConstruct[len(TagConstruct)-1:] == "\"" || TagConstruct[len(TagConstruct)-1:] == "'" {
				TagConstruct = prepareTagName(TagConstruct[1 : len(TagConstruct)-1]) //Cleanup end and beginning quotes
				if sliceContains(ParsedQueryTags, TagConstruct) == false {
					if Negate {
						TagConstruct = "-" + TagConstruct
						Negate = false
					}
					ParsedQueryTags = append(ParsedQueryTags, TagConstruct) //Ensure no dupliccates, add
				}
				//Reset TagConstruct tracking
				TagConstruct = ""
				InQuote = false
			}
		} else if (Tag[0:1] == "\"" && Tag[len(Tag)-1:len(Tag)] == "\"") || (Tag[0:1] == "'" && Tag[len(Tag)-1:len(Tag)] == "'") {
			//Case when tag is already quoted, beggining and ending quotes stripped, then this follows the same as the basic tag. Cleanup, dedupe, add.
			Tag = prepareTagName(Tag[1 : len(Tag)-1]) //Cleanup, remove beginning and ending quotes
			if sliceContains(ParsedQueryTags, Tag) == false {
				if Negate {
					Tag = "-" + Tag
					Negate = false
				}
				ParsedQueryTags = append(ParsedQueryTags, Tag) //Ensure no dupliccates
			}
		} else if Tag[0:1] == "\"" || Tag[0:1] == "'" {
			//If first character of new field/tag is a "
			//We store the tag in a temporary spot until we find the ending "
			InQuote = true
			TagConstruct = Tag
		} else {
			//Default, not in quotes, not starting or ending quotes, just a simple tag or metatag.
			Tag = prepareTagName(Tag) //Cleanup
			if sliceContains(ParsedQueryTags, Tag) == false {
				if Negate {
					Tag = "-" + Tag
					Negate = false
				}
				ParsedQueryTags = append(ParsedQueryTags, Tag) //Ensure no dupliccates
			}
		}
	}
	//Now as a fallback, if TagConstruct has anything in it, treat it as if it ended in a quote
	//For queries formatted like
	//audio "i wrote you a song
	//with this fallback will return
	//audio, i_wrote_you_a_song
	if len(TagConstruct) != 0 {
		//Remove starting quote
		TagConstruct = prepareTagName(TagConstruct[1:]) //Cleanup, remove starting quote
		if sliceContains(ParsedQueryTags, TagConstruct) == false {
			if Negate {
				TagConstruct = "-" + TagConstruct
				Negate = false
			}
			ParsedQueryTags = append(ParsedQueryTags, TagConstruct) //Ensure no dupliccates, add
		}
	}

	//Now set RawQueryTags to our ParsedQueryTags
	RawQueryTags = ParsedQueryTags

	//These are passed to the getTagsInfo function to query SQL
	var IncludeQueryTags []string
	var ExcludeQueryTags []string
	//This stores our pre-toReturn result
	queryMap := make(map[string]interfaces.TagInformation)
	//Loop through each user query tag, and add it to the map, as well as the Exclude/Include subcategories
	for _, v := range RawQueryTags {
		if v[:1] == "-" {
			ExcludeQueryTags = append(ExcludeQueryTags, strings.ToLower(v[1:]))
			//queryMap[strings.ToLower(v[1:])] = interfaces.TagInformation{Name: strings.ToLower(v[1:]), Exclude: true, Exists: false}
		} else if v[:1] == "+" {
			IncludeQueryTags = append(IncludeQueryTags, strings.ToLower(v[1:]))
			//queryMap[strings.ToLower(v[1:])] = interfaces.TagInformation{Name: strings.ToLower(v[1:]), Exclude: false, Exists: false}
		} else {
			IncludeQueryTags = append(IncludeQueryTags, strings.ToLower(v))
			//queryMap[strings.ToLower(v)] = interfaces.TagInformation{Name: strings.ToLower(v), Exclude: false, Exists: false}
		}
	}

	//If we have exclude tags
	if len(ExcludeQueryTags) > 0 {
		//Get more info on them and update querymap with new info
		returnedTags, err := DBConnection.getTagsInfo(ExcludeQueryTags, true, CollectionContext)
		if err != nil {
			return ToReturn, err
		}
		for _, tag := range returnedTags {
			queryMap[tag.Name] = tag
		}
	}
	//If we have include tags
	if len(IncludeQueryTags) > 0 {
		//Get more info on them and add them to the map
		returnedTags, err := DBConnection.getTagsInfo(IncludeQueryTags, false, CollectionContext)
		if err != nil {
			return ToReturn, err
		}
		for _, tag := range returnedTags {
			queryMap[tag.Name] = tag
		}
	}

	//Now query map contains all the data we need. Now we just need to convert it to a slice
	for _, TagInfo := range queryMap {
		ToReturn = append(ToReturn, TagInfo)
	}
	return ToReturn, nil
}

//getTagComparator returns the tagvalue and the comparator, or the original TagValue and an empty string if one does not exist
func getTagComparator(TagValue string) (string, string) {
	tagRunes := []rune(TagValue)
	toReturn := ""
	if tagRunes[0] == '>' || tagRunes[0] == '<' {
		toReturn += string(tagRunes[0])
		tagRunes = tagRunes[1:]
	}
	if tagRunes[0] == '=' {
		toReturn += string(tagRunes[0])
		tagRunes = tagRunes[1:]
	}
	return string(tagRunes), toReturn
}

//getTagsInfo is a helper function to get more details on a set of tags by name, note that the names should be cleaned up before passing to this function.
//This function will also parse Alias mapping and return those, as well as parse meta tags
func (DBConnection *MariaDBPlugin) getTagsInfo(Tags []string, Exclude bool, CollectionContext bool) ([]interfaces.TagInformation, error) {
	//What we will return
	var ToReturn []interfaces.TagInformation
	if len(Tags) == 0 {
		return ToReturn, nil
	}

	//First we handle meta tags
	var NonMetaTags []string //Tags will be set to this and used later on in code
	for _, value := range Tags {
		if strings.Contains(value, ":") {
			MetaValue, Comparator := getTagComparator(strings.Split(value, ":")[1])
			if Comparator == "" {
				Comparator = "="
			}
			ToAdd := interfaces.TagInformation{
				Name:       strings.Split(value, ":")[0],
				MetaValue:  MetaValue,
				Comparator: Comparator,
				Exclude:    Exclude,
				IsMeta:     true}
			ToReturn = append(ToReturn, ToAdd)
		} else {
			NonMetaTags = append(NonMetaTags, value)
		}
	}
	//Parse meta tags further
	//Need to ensure column names are correct, and values too
	if len(ToReturn) > 0 {
		ToReturn, _ = DBConnection.parseMetaTags(ToReturn, CollectionContext)
	}

	Tags = NonMetaTags
	if len(Tags) <= 0 {
		return ToReturn, nil
	}

	//Prepare the dynamic statement. This is safe from SQL injection as we are just dynamically adjusting the placeholder "?s"
	sqlQuery := "SELECT Description, ID, Name, UploaderID, UploadTime, AliasedID, IsAlias FROM Tags WHERE Name IN (?" + strings.Repeat(",?", len(Tags)-1) + ")"
	//Add all the tags into a generic interface to pass to DBQuery
	queryArray := []interface{}{}
	for _, tag := range Tags {
		queryArray = append(queryArray, tag)
	}
	//Pass the sql query to DB
	rows, err := DBConnection.DBHandle.Query(sqlQuery, queryArray...)
	defer rows.Close()
	if err != nil {
		return nil, err
	}

	//Placeholders for data returned by each row
	var Description sql.NullString
	var ID uint64
	var Name string

	var UploaderID uint64
	var NUploadTime mysql.NullTime
	var UploadTime time.Time
	var AliasedID uint64
	var IsAlias bool
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&Description, &ID, &Name, &UploaderID, &NUploadTime, &AliasedID, &IsAlias)
		if err != nil {
			return nil, err
		}
		//If description is a valid non-null value, use it, else, use """
		var SDescription string
		if Description.Valid {
			SDescription = Description.String
		}
		//Get UploadTime if set
		if NUploadTime.Valid {
			UploadTime = NUploadTime.Time
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.TagInformation{Name: Name, ID: ID, Description: SDescription, Exists: true, Exclude: Exclude, UploaderID: UploaderID, UploadTime: UploadTime, AliasedID: AliasedID, IsAlias: IsAlias})
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}

	//Add back in non-existant tags
	for _, tag := range Tags {
		if tagsContainName(tag, ToReturn) == false {
			ToReturn = append(ToReturn, interfaces.TagInformation{
				Name:    tag,
				Exists:  false,
				Exclude: Exclude})
		}
	}

	//Parse alaises
	var AliasedIDs []uint64
	for index := 0; index < len(ToReturn); index++ {
		if ToReturn[index].IsAlias && tagsContainID(ToReturn[index].AliasedID, ToReturn) == false {
			AliasedIDs = append(AliasedIDs, ToReturn[index].AliasedID)
		}
	}

	if len(AliasedIDs) > 0 {
		//Loop through our alias IDs, and add them to ToReturn
		sqlQuery = "SELECT Description, ID, Name, UploaderID, UploadTime, AliasedID, IsAlias FROM Tags WHERE ID IN (?" + strings.Repeat(",?", len(AliasedIDs)-1) + ")"
		//Add all the tags into a generic interface to pass to DBQuery
		queryArray = []interface{}{}
		for _, ID := range AliasedIDs {
			queryArray = append(queryArray, ID)
		}
		//Pass the sql query to DB
		idrows, err := DBConnection.DBHandle.Query(sqlQuery, queryArray...)
		defer idrows.Close()
		if err != nil {
			return nil, err
		}
		//For each row
		for idrows.Next() {
			//Parse out the data
			err := idrows.Scan(&Description, &ID, &Name, &UploaderID, &NUploadTime, &AliasedID, &IsAlias)
			if err != nil {
				return nil, err
			}
			//If description is a valid non-null value, use it, else, use ""
			var SDescription string
			if Description.Valid {
				SDescription = Description.String
			}
			//Get UploadTime if set
			if NUploadTime.Valid {
				UploadTime = NUploadTime.Time
			}
			//Add this result to ToReturn
			ToReturn = append(ToReturn, interfaces.TagInformation{Name: Name, ID: ID, Description: SDescription, Exists: true, Exclude: Exclude, UploaderID: UploaderID, UploadTime: UploadTime, AliasedID: AliasedID, IsAlias: IsAlias})
		}

		err = idrows.Err()
		if err != nil {
			return nil, err
		}
	}

	//Pass output
	return ToReturn, nil
}

//parseMetaTags fills in additional information for MetaTags and vets out non-MetaTags
func (DBConnection *MariaDBPlugin) parseMetaTags(MetaTags []interfaces.TagInformation, CollectionContext bool) ([]interfaces.TagInformation, []error) {
	var ToReturn []interfaces.TagInformation
	var ErrorList []error
	for _, tag := range MetaTags {
		ToAdd := tag
		switch {
		//TODO: Add additional metatags here
		case ToAdd.Name == "uploader":
			ToAdd.Name = "UploaderID"
			ToAdd.Description = "The uploaded of the image"
			//Get uploader ID and set that to value
			name, isString := ToAdd.MetaValue.(string)
			if isString {
				value, err := DBConnection.GetUserID(name)
				if err != nil {
					ErrorList = append(ErrorList, err)
				} else {
					ToAdd.MetaValue = value
					ToAdd.Exists = true
				}
				ToAdd.Comparator = "=" //Clobber any other comparator requested. This one will only support equals
			} else {
				ErrorList = append(ErrorList, errors.New("Could not convert metatag value to string as expected"))
			}
		case ToAdd.Name == "rating" && CollectionContext == false:
			ToAdd.Name = "Rating"
			ToAdd.Description = "The rating of the image"
			ToAdd.Exists = true
			ToAdd.Comparator = "=" //Clobber any other comparator requested. This one will only support equals
			//Since rating is a string, no futher processing needed!
		case ToAdd.Name == "score" && CollectionContext == false:
			ToAdd.Name = "ScoreAverage"
			ToAdd.Description = "The average voted score of the image"
			sscore, isString := ToAdd.MetaValue.(string)
			if isString {
				score, err := strconv.ParseInt(sscore, 10, 64)
				if err == nil {
					ToAdd.MetaValue = score
				}
			}
			//Must be an int64
			_, isInt := ToAdd.MetaValue.(int64)
			if isInt {
				ToAdd.Exists = true
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse requested score, ensure it is a number"))
			}
			//All comparators valid
		case ToAdd.Name == "averagescore" && CollectionContext == false:
			ToAdd.Name = "ScoreAverage"
			ToAdd.Description = "The average voted score of the image"
			sscore, isString := ToAdd.MetaValue.(string)
			if isString {
				score, err := strconv.ParseInt(sscore, 10, 64)
				if err == nil {
					ToAdd.MetaValue = score
				}
			}
			//Must be an int64
			_, isInt := ToAdd.MetaValue.(int64)
			if isInt {
				ToAdd.Exists = true
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse requested score, ensure it is a number"))
			}
			//All comparators valid
		case ToAdd.Name == "totalscore" && CollectionContext == false:
			ToAdd.Name = "ScoreTotal"
			ToAdd.Description = "The total sum of all voted scores for the image"
			sscore, isString := ToAdd.MetaValue.(string)
			if isString {
				score, err := strconv.ParseInt(sscore, 10, 64)
				if err == nil {
					ToAdd.MetaValue = score
				}
			}
			//Must be an int64
			_, isInt := ToAdd.MetaValue.(int64)
			if isInt {
				ToAdd.Exists = true
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse requested score, ensure it is a number"))
			}
			//All comparators valid
		case ToAdd.Name == "scorevoters" && CollectionContext == false:
			ToAdd.Name = "ScoreVoters"
			ToAdd.Description = "The count of all users that voted on the image"
			sscore, isString := ToAdd.MetaValue.(string)
			if isString {
				score, err := strconv.ParseInt(sscore, 10, 64)
				if err == nil {
					ToAdd.MetaValue = score
				}
			}
			//Must be an int64
			_, isInt := ToAdd.MetaValue.(int64)
			if isInt {
				ToAdd.Exists = true
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse requested score, ensure it is a number"))
			}
			//All comparators valid
		case ToAdd.Name == "incollection" && CollectionContext == false:
			ToAdd.Name = "InCollection"
			ToAdd.Description = "Whether the image is in a collection or not"
			ToAdd.IsComplexMeta = true
			inCollOption, isString := ToAdd.MetaValue.(string)
			if isString {
				if inCollOption == "Y" || inCollOption == "y" || inCollOption == "true" {
					ToAdd.MetaValue = true
					ToAdd.Exists = true
				} else if inCollOption == "N" || inCollOption == "n" || inCollOption == "false" {
					ToAdd.MetaValue = false
					ToAdd.Exists = true
				} else {
					ErrorList = append(ErrorList, errors.New("could not parse incollection tag"))
				}
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse incollection tag"))
			}
			ToAdd.Comparator = "=" //Clobber any other comparator requested. This one will only support equals
		case ToAdd.Name == "tagcount" && CollectionContext == false:
			ToAdd.Name = "TagCount"
			ToAdd.Description = "Number of tags an image has"
			ToAdd.IsComplexMeta = true
			stringValue, isString := ToAdd.MetaValue.(string)
			if isString {
				countValue, err := strconv.ParseInt(stringValue, 10, 64)
				if err == nil {
					ToAdd.Exists = true
					ToAdd.MetaValue = strconv.FormatInt(countValue, 10)
				}
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse tagcount tag"))
			}
		case ToAdd.Name == "name":
			ToAdd.Name = "Name"
			ToAdd.Description = "Name of the item"
			ToAdd.IsComplexMeta = false
			inCollOption, isString := ToAdd.MetaValue.(string)
			if isString {

				//This chunk is ugly, but allows us to escape spaces //TODO: This is stupid and needs fixing, and a dedicated function to do so
				inCollOption = strings.Replace(inCollOption, "--", "#", -1) //Placeholder for dash
				inCollOption = strings.Replace(inCollOption, "-_", "$", -1) //Placeholder for underscore
				inCollOption = strings.Replace(inCollOption, "__", " ", -1)
				inCollOption = strings.Replace(inCollOption, "#", "-", -1)
				inCollOption = strings.Replace(inCollOption, "_", "$", -1)
				inCollOption = strings.Replace(inCollOption, "$", "\\_", -1)
				if len(inCollOption) > 3 {
					ToAdd.MetaValue = "%" + inCollOption + "%"
					ToAdd.Exists = true
				} else {
					ErrorList = append(ErrorList, errors.New("could not parse name tag, please lengthen your query"))
				}
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse name tag"))
			}
			ToAdd.Comparator = "LIKE" //Clobber any other comparator requested. This one will only support LIKE
		case ToAdd.Name == "location":
			ToAdd.Name = "Location"
			ToAdd.Description = "The item's file location/name"
			ToAdd.IsComplexMeta = false
			inCollOption, isString := ToAdd.MetaValue.(string)
			if isString {
				//This chunk is ugly, but allows us to escape spaces
				inCollOption = strings.Replace(inCollOption, "--", "#", -1) //Placeholder for dash
				inCollOption = strings.Replace(inCollOption, "-_", "$", -1) //Placeholder for underscore
				inCollOption = strings.Replace(inCollOption, "__", " ", -1)
				inCollOption = strings.Replace(inCollOption, "#", "-", -1)
				inCollOption = strings.Replace(inCollOption, "_", "$", -1)
				inCollOption = strings.Replace(inCollOption, "$", "\\_", -1)
				if len(inCollOption) > 3 {
					ToAdd.MetaValue = "%" + inCollOption + "%"
					ToAdd.Exists = true
				} else {
					ErrorList = append(ErrorList, errors.New("could not parse filename tag, please lengthen your query"))
				}
			} else {
				ErrorList = append(ErrorList, errors.New("could not parse filename tag"))
			}
			ToAdd.Comparator = "LIKE" //Clobber any other comparator requested. This one will only support LIKE
		default:
			ErrorList = append(ErrorList, errors.New("MetaTag does not exist"))
		}
		ToReturn = append(ToReturn, ToAdd)
	}
	return ToReturn, ErrorList
}

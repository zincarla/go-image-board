package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//Tag Operations
var regexTagName = regexp.MustCompile("[^a-zA-Z0-9_-]") //Used to cleanup tag names
var regexWhiteSpace = regexp.MustCompile("\\s{2,}")     //Matches 2 or more consecutive whitespace

func prepareTagName(Name string) string {
	//Lowercase Name -> Trimmed front and end of whitespace -> any inner whitespace reduced and underscored
	Name = regexWhiteSpace.ReplaceAllString(strings.TrimSpace(strings.ToLower(Name)), "_") //Replace all whitespace with _
	//Case of metatag
	if strings.Count(Name, ":") == 1 {
		//Assume a metatag
		NameValue := strings.Split(Name, ":")
		Name = regexTagName.ReplaceAllString(NameValue[0], "_") + ":" + regexTagName.ReplaceAllString(NameValue[1], "_")
	} else {
		//Then any special characters replaced with _
		Name = regexTagName.ReplaceAllString(Name, "_")
	}
	return Name
}

//NewTag adds a tag with the provided information
func (DBConnection *MariaDBPlugin) NewTag(Name string, Description string, UploaderID uint64) (uint64, error) {
	//Cleanup name
	Name = prepareTagName(Name)

	if len(Name) < 3 || len(Name) > 255 || len(Description) > 255 {
		logging.LogInterface.WriteLog("MariaDBPlugin", "NewTag", "*", "ERROR", []string{"Failed to add tag dues to size", Name, Description})
		return 0, errors.New("name or description outside of right sizes")
	}

	resultInfo, err := DBConnection.DBHandle.Exec("INSERT INTO Tags (Name, Description, UploaderID) VALUES (?, ?, ?);", Name, Description, UploaderID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "NewTag", "*", "ERROR", []string{"Failed to add image", err.Error()})
		return 0, err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "NewTag", "*", "SUCCESS", []string{"Image added"})
	id, _ := resultInfo.LastInsertId()
	return uint64(id), err
}

//DeleteTag removes a tag
func (DBConnection *MariaDBPlugin) DeleteTag(TagID uint64) error {
	//Ensure not in use
	var useCount int
	if err := DBConnection.DBHandle.QueryRow("SELECT COUNT(*) AS UseCount FROM ImageTags WHERE TagID = ?", TagID).Scan(&useCount); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteTag", "*", "ERROR", []string{"Tag to delete is still in use", strconv.FormatUint(TagID, 10)})
		return errors.New("tag to delete is still in use")
	}

	//Delete
	_, err := DBConnection.DBHandle.Exec("DELETE FROM Tags WHERE ID=?;", TagID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteTag", "*", "ERROR", []string{"Failed to delete tag", err.Error(), strconv.FormatUint(TagID, 10)})
	} else {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteTag", "*", "SUCCESS", []string{"Tag deleted", strconv.FormatUint(TagID, 10)})
	}
	return err
}

//AddTag adds an association of a tag to image into the association table
func (DBConnection *MariaDBPlugin) AddTag(TagID uint64, ImageID uint64, LinkerID uint64) error {
	//Prevent adding alias
	tagInfo, err := DBConnection.GetTag(TagID)
	if err != nil {
		return errors.New("Failed to validate tag")
	}

	//If this is an alias, then add aliasedid instead
	if tagInfo.IsAlias {
		TagID = tagInfo.AliasedID
	}

	//Ensure image is not already tagged
	var useCount int
	if err := DBConnection.DBHandle.QueryRow("SELECT COUNT(*) AS UseCount FROM ImageTags WHERE TagID = ? AND ImageID = ?", TagID, ImageID).Scan(&useCount); err != nil || useCount > 0 {
		logging.LogInterface.WriteLog("MariaDBPlugin", "AddTag", "*", "ERROR", []string{"Tag to add is already on image", strconv.FormatUint(TagID, 10), strconv.FormatUint(ImageID, 10)})
		return errors.New("tag to add is already on image")
	}

	if _, err := DBConnection.DBHandle.Exec("INSERT INTO ImageTags (TagID, ImageID, LinkerID) VALUES (?, ?, ?);", TagID, ImageID, LinkerID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "AddTag", "*", "ERROR", []string{"Tag not added to image", strconv.FormatUint(TagID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "AddTag", "*", "SUCCESS", []string{"Tag added", strconv.FormatUint(TagID, 10), strconv.FormatUint(ImageID, 10)})
	return nil
}

//GetImageTags returns a list of TagInformation for all tags that apply to the given image
func (DBConnection *MariaDBPlugin) GetImageTags(ImageID uint64) ([]interfaces.TagInformation, error) {
	var ToReturn []interfaces.TagInformation

	//SELECT Tags.ID AS ID, Tags.Name AS Name, Tags.Description AS Description FROM ImageTags INNER JOIN Tags ON Tags.ID = ImageTags.TagID WHERE ImageID=?

	sqlQuery := "SELECT Tags.ID, Tags.Name, Tags.Description FROM ImageTags INNER JOIN Tags ON Tags.ID = ImageTags.TagID WHERE ImageID=?"
	//Pass the sql query to DB
	rows, err := DBConnection.DBHandle.Query(sqlQuery, ImageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	//Placeholders for data returned by each row
	var Description sql.NullString
	var ID uint64
	var Name string
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&ID, &Name, &Description)
		if err != nil {
			return nil, err
		}
		//If description is a valid non-null value, use it, else, use ""
		var SDescription string
		if Description.Valid {
			SDescription = Description.String
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.TagInformation{Name: Name, ID: ID, Description: SDescription, Exists: true, Exclude: false})
	}
	return ToReturn, nil
}

//RemoveTag remove a tag association
func (DBConnection *MariaDBPlugin) RemoveTag(TagID uint64, ImageID uint64) error {
	if _, err := DBConnection.DBHandle.Exec("DELETE FROM ImageTags WHERE TagID=? AND ImageID=?;", TagID, ImageID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "RemoveTag", "*", "WARN", []string{"Tag to remove was not on image", strconv.FormatUint(TagID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "RemoveTag", "*", "SUCCESS", []string{"Tag removed", strconv.FormatUint(TagID, 10), strconv.FormatUint(ImageID, 10)})
	return nil
}

//GetQueryTags returns a slice of tags based on a query string
func (DBConnection *MariaDBPlugin) GetQueryTags(UserQuery string) ([]interfaces.TagInformation, error) {
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

	//If we have exlude tags
	if len(ExcludeQueryTags) > 0 {
		//Get more info on them and update querymap with new info
		returnedTags, err := DBConnection.getTagsInfo(ExcludeQueryTags, true)
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
		returnedTags, err := DBConnection.getTagsInfo(IncludeQueryTags, false)
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

//getTagsInfo is a helper function to get more details on a set of tags by name, note that the names should be cleaned up before passing to this function.
//This function will also parse Alias mapping and return those as well.
func (DBConnection *MariaDBPlugin) getTagsInfo(Tags []string, Exclude bool) ([]interfaces.TagInformation, error) {
	//What we will return
	var ToReturn []interfaces.TagInformation
	if len(Tags) == 0 {
		return ToReturn, nil
	}

	//First we handle meta tags
	var NonMetaTags []string //Tags will be set to this and used later on in code
	for _, value := range Tags {
		if strings.Contains(value, ":") {
			ToAdd := interfaces.TagInformation{
				Name:      strings.Split(value, ":")[0],
				MetaValue: strings.Split(value, ":")[1],
				Exclude:   Exclude,
				IsMeta:    true}
			ToReturn = append(ToReturn, ToAdd)
		} else {
			NonMetaTags = append(NonMetaTags, value)
		}
	}
	//Parse meta tags further
	//Need to ensure column names are correct, and values too
	if len(ToReturn) > 0 {
		ToReturn, _ = DBConnection.parseMetaTags(ToReturn)
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

//tagsContainID is a helper function to check if a TagInformation slice contains a specified ID
func tagsContainID(ID uint64, Tags []interfaces.TagInformation) bool {
	for _, Tag := range Tags {
		if Tag.ID == ID {
			return true
		}
	}
	return false
}

//tagsContainName is a helper function to check if a TagInformation slice contains a specified Name
func tagsContainName(Name string, Tags []interfaces.TagInformation) bool {
	for _, Tag := range Tags {
		if Tag.Name == Name {
			return true
		}
	}
	return false
}

//GetAllTags returns a list of all tags, but only the ID, Name, Description, and IsAlias
func (DBConnection *MariaDBPlugin) GetAllTags() ([]interfaces.TagInformation, error) {
	var ToReturn []interfaces.TagInformation

	sqlQuery := "SELECT ID, Name, Description, IsAlias FROM Tags ORDER BY Name"
	//Pass the sql query to DB
	rows, err := DBConnection.DBHandle.Query(sqlQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	//Placeholders for data returned by each row
	var Description sql.NullString
	var ID uint64
	var Name string
	var IsAlias bool
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&ID, &Name, &Description, &IsAlias)
		if err != nil {
			return nil, err
		}
		//If description is a valid non-null value, use it, else, use ""
		var SDescription string
		if Description.Valid {
			SDescription = Description.String
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.TagInformation{Name: Name, ID: ID, Description: SDescription, Exists: true, Exclude: false, IsAlias: IsAlias})
	}
	return ToReturn, nil
}

//GetTag returns detailed information on one tag
func (DBConnection *MariaDBPlugin) GetTag(ID uint64) (interfaces.TagInformation, error) {
	sqlQuery := "SELECT Name, Description, UploaderID, UploadTime, AliasedID, IsAlias FROM Tags WHERE ID=?"
	//Pass the sql query to DB
	//Placeholders for data returned by each row
	var Description sql.NullString
	var Name string
	var UploaderID uint64
	var NUploadTime mysql.NullTime
	var UploadTime time.Time
	var AliasedID uint64
	var IsAlias bool
	err := DBConnection.DBHandle.QueryRow(sqlQuery, ID).Scan(&Name, &Description, &UploaderID, &NUploadTime, &AliasedID, &IsAlias)
	if err != nil {
		return interfaces.TagInformation{ID: ID, Exists: false}, err
	}
	//If description is a valid non-null value, use it, else, use ""
	var SDescription string
	if Description.Valid {
		SDescription = Description.String
	}

	if NUploadTime.Valid {
		UploadTime = NUploadTime.Time
	}

	return interfaces.TagInformation{Name: Name, ID: ID, Description: SDescription, Exists: true, Exclude: false, UploaderID: UploaderID, UploadTime: UploadTime, AliasedID: AliasedID, IsAlias: IsAlias}, nil
}

//UpdateTag updates a pre-existing tag
func (DBConnection *MariaDBPlugin) UpdateTag(TagID uint64, Name string, Description string, AliasedID uint64, IsAlias bool, RequestorID uint64) error {
	//Cleanup name
	Name = prepareTagName(Name)
	if len(Name) < 3 || len(Name) > 255 || len(Description) > 255 {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateTag", "*", "ERROR", []string{"Failed to update tag dues to size", Name, Description})
		return errors.New("name or description outside of right sizes")
	}

	if IsAlias {
		//Prevent adding alias
		tagInfo, err := DBConnection.GetTag(AliasedID)
		if err != nil || tagInfo.IsAlias {
			return errors.New("Tag to alias could not be found, or is an alias itself")
		}
	}

	_, err := DBConnection.DBHandle.Exec("UPDATE Tags SET Name = ?, Description=?, AliasedID=?, IsAlias=? WHERE ID=?;", Name, Description, AliasedID, IsAlias, TagID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateTag", "*", "ERROR", []string{"Failed to update tag", err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateTag", "*", "SUCCESS", []string{"Image added"})

	if IsAlias {
		go DBConnection.ReplaceImageTags(TagID, AliasedID, RequestorID)
	}

	return nil
}

//ReplaceImageTags replaces all instances of ImageTags that have the specified tag with the new tag
func (DBConnection *MariaDBPlugin) ReplaceImageTags(OldTagID uint64, NewTagID uint64, LinkerID uint64) error {
	query := `UPDATE ImageTags
	SET TagID = ? , LinkerID=?
	WHERE TagID=? AND ImageID NOT IN
	(
	  SELECT ImageID
	  FROM (SELECT ImageID from ImageTags WHERE TagID=?) AS InnerTags
	);`
	_, err := DBConnection.DBHandle.Exec(query, NewTagID, LinkerID, OldTagID, NewTagID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "ReplaceImageTags", "*", "ERROR", []string{"Failed to update imagetags", err.Error()})
		return err
	}
	//Remove any instances of old tag, first query replaces the old tag on all images, but does not allow duplicates. This query will remove the old tag that would have been replaced if it would not have lead to a duplicate.
	_, err = DBConnection.DBHandle.Exec("DELETE FROM ImageTags WHERE TagID=?;", OldTagID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "ReplaceImageTags", "*", "ERROR", []string{"Failed to remove old instances of tag", err.Error()})
		return err
	}
	return nil
}

//BulkAddTag adds an association of a tag to image into the association table that already have another tag
func (DBConnection *MariaDBPlugin) BulkAddTag(TagID uint64, OldTagID uint64, LinkerID uint64) error {
	//Prevent adding alias
	tagInfo, err := DBConnection.GetTag(TagID)
	oldTagInfo, err2 := DBConnection.GetTag(OldTagID)
	if err != nil || err2 != nil {
		return errors.New("Failed to validate tags")
	}

	//If this is an alias, then add aliasedid instead
	if tagInfo.IsAlias {
		TagID = tagInfo.AliasedID
	}

	//Similiarly convert oldTag if it is an alias
	if oldTagInfo.IsAlias {
		OldTagID = oldTagInfo.AliasedID
	}

	if _, err := DBConnection.DBHandle.Exec("INSERT INTO ImageTags (TagID, ImageID, LinkerID) SELECT ?, ImageID, ? FROM ImageTags WHERE TagID=? AND ImageID NOT IN (SELECT ImageID FROM ImageTags WHERE TagID=?);", TagID, LinkerID, OldTagID, TagID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "BulkAddTag", "*", "ERROR", []string{"Tag not added to image", strconv.FormatUint(OldTagID, 10), strconv.FormatUint(TagID, 10), err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "BulkAddTag", "*", "SUCCESS", []string{"Tags added", strconv.FormatUint(OldTagID, 10), strconv.FormatUint(TagID, 10)})
	return nil
}

//sliceContains is a helper function that returns whether a slice contains a specifc string
func sliceContains(slice []string, item string) bool {
	for _, sliceItem := range slice {
		if sliceItem == item {
			return true
		}
	}
	return false
}

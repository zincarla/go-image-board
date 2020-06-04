package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"math/rand"
	"strconv"
	"strings"
)

//SearchImages performs a search for images (Returns a list of ImageInformations a result count and an error/nil)
//If you edit this function, consider SearchCollections and GetPrevNexImages for a similar change
func (DBConnection *MariaDBPlugin) SearchImages(Tags []interfaces.TagInformation, PageStart uint64, PageStride uint64) ([]interfaces.ImageInformation, uint64, error) {
	//Cleanup input for use in code below
	//Specifically we separate the include, the exclude and metatags into their own lists
	var IncludeTags []uint64
	var ExcludeTags []uint64
	var MetaTags []interfaces.TagInformation
	for _, tag := range Tags {
		if tag.Exists && tag.IsAlias == false && tag.IsMeta == false {
			if tag.Exclude {
				ExcludeTags = append(ExcludeTags, tag.ID)
			} else {
				IncludeTags = append(IncludeTags, tag.ID)
			}
		} else if tag.Exists && tag.IsMeta {
			MetaTags = append(MetaTags, tag)
		}
	}

	//Initialize output
	var ToReturn []interfaces.ImageInformation
	var MaxResults uint64

	//Construct SQL Query

	//This is the start of the query we want
	sqlQuery := `SELECT ID, Name, Location `
	sqlCountQuery := `SELECT COUNT(*) `
	orderClause := `ORDER BY ID DESC LIMIT ? OFFSET ?;`
	if len(IncludeTags) == 0 {
		sqlQuery = `SELECT Images.ID, Name, Location ` + `FROM Images JOIN ImagedHashes ON Images.ID=ImagedHashes.ImageID `
		sqlCountQuery = sqlCountQuery + `FROM Images `
	} else {
		sqlQuery = sqlQuery + `FROM (
			SELECT ImageTags.ImageID as ID, Images.Name, Images.Location, COUNT(*) as MatchingTags, ImagedHashes.hHash, ImagedHashes.vHash
			FROM ImageTags 
			INNER JOIN Images ON ImageTags.ImageID=Images.ID JOIN ImagedHashes ON ImageTags.ImageID=ImagedHashes.ImageID `
		sqlCountQuery = sqlCountQuery + `FROM ( 
			SELECT ImageID as ID, Name, Location, COUNT(*) as MatchingTags
			FROM ImageTags 
			INNER JOIN Images ON ImageTags.ImageID=Images.ID `
	}

	//Now for the variable piece
	sqlWhereClause := ""
	if len(IncludeTags) > 0 {
		sqlWhereClause = sqlWhereClause + "WHERE TagID IN (?" + strings.Repeat(",?", len(IncludeTags)-1) + ") "
	}
	if len(ExcludeTags) > 0 {
		if len(IncludeTags) > 0 {
			sqlWhereClause += "AND "
		} else {
			sqlWhereClause += "WHERE "
		}
		sqlWhereClause += "Images.ID NOT IN (SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (?" + strings.Repeat(",?", len(ExcludeTags)-1) + ")) "
	}

	//And add any metatags
	if len(MetaTags) > 0 {
		for _, tag := range MetaTags {
			metaTagQuery := "AND "
			if sqlWhereClause == "" {
				metaTagQuery = "WHERE "
			}

			//Handle Comparator transforms
			comparator := tag.Comparator
			if tag.Exclude {
				comparator = getInvertedComparator(comparator)
			}
			if comparator == "" {
				return ToReturn, 0, errors.New("Failed to invert query to negate on " + tag.Name)
			}

			//Handle Complex Tags Here
			if tag.Name == "InCollection" { //Special Exception for InCollection
				tagBoolValue, isTagValued := tag.MetaValue.(bool)
				if isTagValued == false {
					return ToReturn, 0, errors.New("Failed get value of " + tag.Name)
				}
				if (comparator == "=" && tagBoolValue == true) || (comparator == "!=" && tagBoolValue == false) {
					comparator = " IN "
				} else {
					comparator = " NOT IN "
				}
				metaTagQuery += "Images.ID" + comparator + "(SELECT DISTINCT ImageID FROM CollectionMembers) "
				sqlWhereClause = sqlWhereClause + metaTagQuery
				continue //Skip over rest of code for this tag
			} else if tag.Name == "TagCount" { //Special Exception for TagCount
				tagStringValue, isTagValued := tag.MetaValue.(string)
				if isTagValued == false {
					return ToReturn, 0, errors.New("Failed get value of " + tag.Name)
				}
				metaTagQuery += "Images.ID IN (SELECT TagCountTBL.ImageID FROM (SELECT ImageID, COUNT(*) AS TagCount FROM `ImageTags` GROUP BY ImageTags.ImageID) TagCountTBL WHERE TagCountTBL.TagCount " + comparator + " " + tagStringValue + ") "
				sqlWhereClause = sqlWhereClause + metaTagQuery
				continue //Skip over rest of code for this tag
			} else if tag.Name == "OrderSimiliar" {
				tagHashValues, isTagValued := tag.MetaValue.(interfaces.ImagedHash)
				if isTagValued == false {
					return ToReturn, 0, errors.New("Failed get value of " + tag.Name)
				}
				orderClause = `ORDER BY (BIT_COUNT(hHash ^ ` + strconv.FormatUint(tagHashValues.ImagehHash, 10) + `)+BIT_COUNT(vHash ^ ` + strconv.FormatUint(tagHashValues.ImagevHash, 10) + `)) ASC LIMIT ? OFFSET ?;`
				continue
			}

			metaTagQuery = metaTagQuery + "Images." + tag.Name + " "
			metaTagQuery = metaTagQuery + comparator + " ? "
			sqlWhereClause = sqlWhereClause + metaTagQuery
		}
	}

	if len(IncludeTags) > 0 {
		sqlQuery = sqlQuery + sqlWhereClause + `GROUP BY ImageTags.ImageID) InnerStatement WHERE MatchingTags = ? `
		sqlCountQuery = sqlCountQuery + sqlWhereClause + `GROUP BY ImageTags.ImageID) InnerStatement WHERE MatchingTags = ? `
	} else {
		sqlQuery = sqlQuery + sqlWhereClause
		sqlCountQuery = sqlCountQuery + sqlWhereClause
	}

	//Add Order
	sqlQuery = sqlQuery + orderClause

	//Now construct arguments list. Order must follow query order
	/*
		Inclusive Tags
		Exclusive Tags
		Inclusive Tag Count
		<However we pause here to run count query, as that one does not have limits>
		Max Amount of results to return (Stride)
		Offset (Start)
	*/
	queryArray := []interface{}{}
	//Add inclusive tags to our queryArray
	for _, tag := range IncludeTags {
		queryArray = append(queryArray, tag)
	}
	//Add the exclusive tags
	for _, tag := range ExcludeTags {
		queryArray = append(queryArray, tag)
	}
	//Add values for metatags
	for _, tag := range MetaTags {
		//Handle Complex Tags Here
		if tag.Name == "InCollection" || tag.Name == "TagCount" || tag.Name == "OrderSimiliar" { //Special Exception for cert MetaTags
			continue
		}
		//Otherwise use default
		queryArray = append(queryArray, tag.MetaValue)
	}

	//Add inclusive tag count, but only if we have any
	if len(IncludeTags) > 0 {
		queryArray = append(queryArray, len(IncludeTags))
	}

	//Run the count query (Count query does not use start/stride, so run this before we add those)
	err := DBConnection.DBHandle.QueryRow(sqlCountQuery, queryArray...).Scan(&MaxResults)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "SearchImages", "*", "ERROR", []string{"Error running search query", sqlCountQuery, err.Error()})
		return nil, 0, err
	}

	//Add rest of arguments now that we have max result count
	queryArray = append(queryArray, PageStride)
	queryArray = append(queryArray, PageStart)

	//Now we have query and args, run the query
	rows, err := DBConnection.DBHandle.Query(sqlQuery, queryArray...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	//Placeholders for data returned by each row
	var ImageID uint64
	var Name string
	var Location string
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&ImageID, &Name, &Location)
		if err != nil {
			return nil, 0, err
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.ImageInformation{Name: Name, ID: ImageID, Location: Location})
	}
	return ToReturn, MaxResults, nil
}

//GetPrevNexImages performs a search for images (Returns a list of ImageInformations (Up to 2) and an error/nil)
func (DBConnection *MariaDBPlugin) GetPrevNexImages(Tags []interfaces.TagInformation, TargetID uint64) ([]interfaces.ImageInformation, error) {
	if TargetID == 0 {
		return nil, errors.New("invalid targetid")
	}

	var ToReturn []interfaces.ImageInformation

	if imageInfo, err := DBConnection.getPrevNexImage(Tags, TargetID, true); err == nil {
		ToReturn = append(ToReturn, imageInfo)
	} else if err != sql.ErrNoRows {
		return ToReturn, err
	}

	if imageInfo, err := DBConnection.getPrevNexImage(Tags, TargetID, false); err == nil {
		ToReturn = append(ToReturn, imageInfo)
	} else if err != sql.ErrNoRows {
		return ToReturn, err
	}

	return ToReturn, nil
}

//GetPrevNexImages performs a search for images (Returns a ImageInformation and an error/nil)
func (DBConnection *MariaDBPlugin) getPrevNexImage(Tags []interfaces.TagInformation, TargetID uint64, Next bool) (interfaces.ImageInformation, error) {
	//Cleanup input for use in code below
	//Specifically we separate the include, the exclude and metatags into their own lists
	var IncludeTags []uint64
	var ExcludeTags []uint64
	var MetaTags []interfaces.TagInformation
	for _, tag := range Tags {
		if tag.Exists && tag.IsAlias == false && tag.IsMeta == false {
			if tag.Exclude {
				ExcludeTags = append(ExcludeTags, tag.ID)
			} else {
				IncludeTags = append(IncludeTags, tag.ID)
			}
		} else if tag.Exists && tag.IsMeta {
			MetaTags = append(MetaTags, tag)
		}
	}

	//Initialize output
	var ToReturn interfaces.ImageInformation
	//var MaxResults uint64

	//Construct SQL Query

	//This is the start of the query we want
	sqlQuery := `SELECT ID, Name, Location `
	sqlCountQuery := `SELECT COUNT(*) `
	if len(IncludeTags) == 0 {
		sqlQuery = sqlQuery + `FROM Images `
		sqlCountQuery = sqlCountQuery + `FROM Images `
	} else {
		sqlQuery = sqlQuery + `FROM (
			SELECT ImageID as ID, Name, Location, COUNT(*) as MatchingTags
			FROM ImageTags 
			INNER JOIN Images ON ImageTags.ImageID=Images.ID `
		sqlCountQuery = sqlCountQuery + `FROM ( 
			SELECT ImageID as ID, Name, Location, COUNT(*) as MatchingTags
			FROM ImageTags 
			INNER JOIN Images ON ImageTags.ImageID=Images.ID `
	}

	//Now for the variable piece
	sqlWhereClause := ""
	if len(IncludeTags) > 0 {
		sqlWhereClause = sqlWhereClause + "WHERE TagID IN (?" + strings.Repeat(",?", len(IncludeTags)-1) + ") "
	}
	if len(ExcludeTags) > 0 {
		if len(IncludeTags) > 0 {
			sqlWhereClause += "AND "
		} else {
			sqlWhereClause += "WHERE "
		}
		sqlWhereClause += "Images.ID NOT IN (SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (?" + strings.Repeat(",?", len(ExcludeTags)-1) + ")) "
	}

	//And add any metatags
	if len(MetaTags) > 0 {
		for _, tag := range MetaTags {
			metaTagQuery := "AND "
			if sqlWhereClause == "" {
				metaTagQuery = "WHERE "
			}

			//Handle Comparator transforms
			comparator := tag.Comparator
			if tag.Exclude {
				comparator = getInvertedComparator(comparator)
			}
			if comparator == "" {
				return ToReturn, errors.New("Failed to invert query to negate on " + tag.Name)
			}

			//Handle Complex Tags Here
			if tag.Name == "InCollection" { //Special Exception for InCollection
				tagBoolValue, isTagValued := tag.MetaValue.(bool)
				if isTagValued == false {
					return ToReturn, errors.New("Failed get value of " + tag.Name)
				}
				if (comparator == "=" && tagBoolValue == true) || (comparator == "!=" && tagBoolValue == false) {
					comparator = " IN "
				} else {
					comparator = " NOT IN "
				}
				metaTagQuery += "Images.ID" + comparator + "(SELECT DISTINCT ImageID FROM CollectionMembers) "
				sqlWhereClause = sqlWhereClause + metaTagQuery
				continue //Skip over rest of code for this tag
			} else if tag.Name == "TagCount" { //Special Exception for TagCount
				tagStringValue, isTagValued := tag.MetaValue.(string)
				if isTagValued == false {
					return ToReturn, errors.New("Failed get value of " + tag.Name)
				}
				metaTagQuery += "Images.ID IN (SELECT ImageID FROM (SELECT ImageID, COUNT(*) AS TagCount FROM `ImageTags` GROUP BY ImageID) TagCountTBL WHERE TagCountTBL.TagCount " + comparator + " " + tagStringValue + ") "
				sqlWhereClause = sqlWhereClause + metaTagQuery
				continue //Skip over rest of code for this tag
			}

			metaTagQuery = metaTagQuery + "Images." + tag.Name + " "
			metaTagQuery = metaTagQuery + comparator + " ? "
			sqlWhereClause = sqlWhereClause + metaTagQuery
		}
	}

	//Add changes for next/prev
	if sqlWhereClause == "" {
		sqlWhereClause += "WHERE "
	} else {
		sqlWhereClause += "AND "
	}
	if Next == false {
		sqlWhereClause += "Images.ID < ? "
	} else {
		sqlWhereClause += "Images.ID > ? "
	}

	if len(IncludeTags) > 0 {
		sqlQuery = sqlQuery + sqlWhereClause + `GROUP BY ImageID) InnerStatement WHERE MatchingTags = ? `
		sqlCountQuery = sqlCountQuery + sqlWhereClause + `GROUP BY ImageID) InnerStatement WHERE MatchingTags = ? `
	} else {
		sqlQuery = sqlQuery + sqlWhereClause
		sqlCountQuery = sqlCountQuery + sqlWhereClause
	}

	//Add Order
	order := "DESC "
	if Next {
		order = ""
	}
	sqlQuery = sqlQuery + `ORDER BY ID ` + order + `LIMIT 1;`

	//Now construct arguments list. Order must follow query order
	/*
		Inclusive Tags
		Exclusive Tags
		Inclusive Tag Count
		<However we pause here to run count query, as that one does not have limits>
		Max Amount of results to return (Stride)
		Offset (Start)
	*/
	queryArray := []interface{}{}
	//Add inclusive tags to our queryArray
	for _, tag := range IncludeTags {
		queryArray = append(queryArray, tag)
	}
	//Add the exclusive tags
	for _, tag := range ExcludeTags {
		queryArray = append(queryArray, tag)
	}
	//Add values for metatags
	for _, tag := range MetaTags {
		//Handle Complex Tags Here
		if tag.Name == "InCollection" || tag.Name == "TagCount" { //Special Exception for cert MetaTags
			continue
		}
		//Otherwise use default
		queryArray = append(queryArray, tag.MetaValue)
	}

	//Add ID
	queryArray = append(queryArray, TargetID)

	//Add inclusive tag count, but only if we have any
	if len(IncludeTags) > 0 {
		queryArray = append(queryArray, len(IncludeTags))
	}

	//Run the count query (Count query does not use start/stride, so run this before we add those)
	/*err := DBConnection.DBHandle.QueryRow(sqlCountQuery, queryArray...).Scan(&MaxResults) //Uneeded
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "SearchImages", "*", "ERROR", []string{"Error running search query", sqlCountQuery, err.Error()})
		return nil, 0, err
	}*/

	/*Add rest of arguments now that we have max result count
	queryArray = append(queryArray, PageStride)
	queryArray = append(queryArray, PageStart)*/

	//Placeholders for data returned by each row
	var ImageID uint64
	var Name string
	var Location string

	//Now we have query and args, run the query
	err := DBConnection.DBHandle.QueryRow(sqlQuery, queryArray...).Scan(&ImageID, &Name, &Location)
	if err != nil {
		return ToReturn, err
	}
	ToReturn = interfaces.ImageInformation{Name: Name, ID: ImageID, Location: Location}

	return ToReturn, nil
}

//GetRandomImage returns a random image (Returns a ImageInformation and an error/nil)
func (DBConnection *MariaDBPlugin) GetRandomImage(Tags []interfaces.TagInformation) (interfaces.ImageInformation, error) {
	imageInfo, resultCount, err := DBConnection.SearchImages(Tags, 0, 1)

	if err == nil {
		if resultCount <= 0 {
			return interfaces.ImageInformation{}, errors.New("no images found with provided tags")
		}
		if resultCount == 1 {
			return imageInfo[0], nil //Shortcut for one result
		}

		rando := rand.Float64()
		randoID := uint64(rando * float64(resultCount))
		imageInfo, _, err = DBConnection.SearchImages(Tags, randoID, 1)
		if err == nil {
			return imageInfo[0], nil
		}
		return interfaces.ImageInformation{}, err
	}
	return interfaces.ImageInformation{}, err
}

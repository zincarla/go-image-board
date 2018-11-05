package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

//Image operations

//NewImage adds an image with the provided information
func (DBConnection *MariaDBPlugin) NewImage(ImageName string, ImageFileName string, OwnerID uint64, Source string) (uint64, error) {
	resultInfo, err := DBConnection.DBHandle.Exec("INSERT INTO Images (Name, Location, UploaderID, Source) VALUES (?, ?, ?, ?);", ImageName, ImageFileName, OwnerID, Source)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "NewImage", "*", "ERROR", []string{"Failed to add image", err.Error()})
		return 0, err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "NewImage", "*", "SUCCESS", []string{"Image added"})
	id, _ := resultInfo.LastInsertId()
	return uint64(id), err
}

//DeleteImage removes an image from the db
func (DBConnection *MariaDBPlugin) DeleteImage(ImageID uint64) error {
	//First delete ImageTags
	_, err := DBConnection.DBHandle.Exec("DELETE FROM ImageTags WHERE ImageID=?;", ImageID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteImage", "*", "ERROR", []string{"Failed to delete image", err.Error(), strconv.FormatUint(ImageID, 10)})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteImage", "*", "SUCCESS", []string{"Image tags deleted", strconv.FormatUint(ImageID, 10)})
	//Second delete Image from table
	_, err = DBConnection.DBHandle.Exec("DELETE FROM Images WHERE ID=?;", ImageID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteImage", "*", "ERROR", []string{"Failed to delete image", err.Error(), strconv.FormatUint(ImageID, 10)})
	} else {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteImage", "*", "SUCCESS", []string{"Image deleted", strconv.FormatUint(ImageID, 10)})
	}
	return err
}

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
			}

			metaTagQuery = metaTagQuery + "Images." + tag.Name + " "
			metaTagQuery = metaTagQuery + comparator + " ? "
			sqlWhereClause = sqlWhereClause + metaTagQuery
		}
	}

	if len(IncludeTags) > 0 {
		sqlQuery = sqlQuery + sqlWhereClause + `GROUP BY ImageID) InnerStatement WHERE MatchingTags = ? `
		sqlCountQuery = sqlCountQuery + sqlWhereClause + `GROUP BY ImageID) InnerStatement WHERE MatchingTags = ? `
	} else {
		sqlQuery = sqlQuery + sqlWhereClause
		sqlCountQuery = sqlCountQuery + sqlWhereClause
	}

	//Add Order
	sqlQuery = sqlQuery + `ORDER BY ID DESC LIMIT ? OFFSET ?;`

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
		if tag.Name == "InCollection" { //Special Exception for InCollection
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

//GetPrevNexImages performs a search for images (Returns a list of ImageInformations (Up to 2) and an error/nil)
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
		if tag.Name == "InCollection" { //Special Exception for InCollection
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

//GetImage returns information on a single image (Returns an ImageInformation, or error)
func (DBConnection *MariaDBPlugin) GetImage(ID uint64) (interfaces.ImageInformation, error) {
	ToReturn := interfaces.ImageInformation{ID: ID}
	var UploadTime mysql.NullTime
	err := DBConnection.DBHandle.QueryRow("Select Images.Name as ImageName, Images.Location, Images.UploaderID, Images.UploadTime, Images.Rating, Users.Name as UploaderName, Images.ScoreAverage, Images.ScoreTotal, Images.ScoreVoters, Images.Source FROM Images LEFT OUTER JOIN Users ON Images.UploaderID = Users.ID WHERE Images.ID=?", ID).Scan(&ToReturn.Name, &ToReturn.Location, &ToReturn.UploaderID, &UploadTime, &ToReturn.Rating, &ToReturn.UploaderName, &ToReturn.ScoreAverage, &ToReturn.ScoreTotal, &ToReturn.ScoreVoters, &ToReturn.Source)
	if err != nil {
		logging.LogInterface.WriteLog("ImageFunctions", "GetImage", "*", "ERROR", []string{"Failed to get image info from database", err.Error()})
		return ToReturn, err
	}
	if UploadTime.Valid {
		ToReturn.UploadTime = UploadTime.Time
	}
	return ToReturn, nil
}

//SetImageRating changes a given image's rating in the database
func (DBConnection *MariaDBPlugin) SetImageRating(ID uint64, Rating string) error {
	_, err := DBConnection.DBHandle.Exec("UPDATE Images SET Rating = ? WHERE ID = ?;", Rating, ID)
	if err != nil {
		logging.LogInterface.WriteLog("ImageFunctions", "SetImageRating", "*", "ERROR", []string{"Failed to set image rating", err.Error()})
		return err
	}
	return nil
}

//SetImageSource changes a given image's source in the database
func (DBConnection *MariaDBPlugin) SetImageSource(ID uint64, Source string) error {
	_, err := DBConnection.DBHandle.Exec("UPDATE Images SET Source = ? WHERE ID = ?;", Source, ID)
	if err != nil {
		logging.LogInterface.WriteLog("ImageFunctions", "SetImageSource", "*", "ERROR", []string{"Failed to set image source", err.Error()})
		return err
	}
	return nil
}

/*
//Our select query, if inclusive
SELECT ImageID, Name, Location FROM (
	SELECT ImageID, Name, Location, Count(*) as MatchingTags
	FROM ImageTags
	INNER JOIN Images ON ImageTags.ImageID=Images.ID
	[WHERE ][TagID IN (1, 2, 3)]
		[AND ][ImageID NOT IN (
								SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (4)
							)]
	GROUP BY ImageID
) InnerStatement
WHERE MatchingTags = 3
ORDER BY ImageID DESC LIMIT 30 OFFSET 0;

//Our Count Query
SELECT COUNT(ImageID) FROM (
	SELECT ImageID, Name, Location, Count(*) as MatchingTags
	FROM ImageTags
	INNER JOIN Images ON ImageTags.ImageID=Images.ID
	WHERE TagID IN (1, 2, 3)
		AND ImageID NOT IN (
								SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (4)
							)
	GROUP BY ImageID
) InnerStatement
WHERE MatchingTags = 3
*/

/*
//Our select query, if blank or exlusive
SELECT ImageID, Name, Location FROM Images [WHERE ][ImageID NOT IN (
		SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (4, 5, 6)
	)]
ORDER BY ImageID DESC LIMIT 30 OFFSET 0;

//Our Count Query
SELECT COUNT(*) FROM Images [WHERE ][ImageID NOT IN (
		SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (4, 5, 6)
	)]
*/

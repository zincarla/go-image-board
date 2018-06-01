package mariadbplugin

import (
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
func (DBConnection *MariaDBPlugin) SearchImages(Tags []interfaces.TagInformation, PageStart uint64, PageStride uint64) ([]interfaces.ImageInformation, uint64, error) {
	//Cleanup input for use in code below
	//Specifically we seperate the include, the exclude and metatags into their own lists
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

	//Short circuit if we have no inclusive tags
	//This makes the query and function simpler to handle
	if len(IncludeTags) == 0 {
		return DBConnection.searchImagesExclusive(ExcludeTags, MetaTags, PageStart, PageStride)
	}
	//Initialize output
	var ToReturn []interfaces.ImageInformation
	var MaxResults uint64

	//Construct SQL Query

	//This is the start of the query we want
	sqlQuery := `SELECT ImageID, Name, Location FROM (
	SELECT ImageID, Name, Location, COUNT(*) as MatchingTags
	FROM ImageTags 
	INNER JOIN Images ON ImageTags.ImageID=Images.ID `

	sqlCountQuery := `SELECT COUNT(ImageID) FROM ( 
	SELECT ImageID, Name, Location, COUNT(*) as MatchingTags
	FROM ImageTags 
	INNER JOIN Images ON ImageTags.ImageID=Images.ID `

	//Now for the variable piece
	sqlWhereClause := "WHERE TagID IN (?" + strings.Repeat(",?", len(IncludeTags)-1) + ") "
	if len(ExcludeTags) > 0 {
		sqlWhereClause = "AND ImageID NOT IN (SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (?" + strings.Repeat(",?", len(ExcludeTags)-1) + ")) "
	}

	//And add any metatags
	if len(MetaTags) > 0 {
		for _, tag := range MetaTags {
			metaTagQuery := "AND Images." + tag.Name + " "
			comparator := tag.Comparator
			if tag.Exclude {
				comparator = getInvertedComparator(comparator)
			}
			if comparator == "" {
				return ToReturn, 0, errors.New("Failed to invert query to negate on " + tag.Name)
			}
			metaTagQuery = metaTagQuery + comparator + " ? "

			sqlWhereClause = sqlWhereClause + metaTagQuery
		}
	}

	//Count of tags, so we only match images that have all the tags we are looking for
	var matchingTagsFilter = "WHERE MatchingTags = ?"

	//Finish off query
	sqlQuery = sqlQuery + sqlWhereClause + `GROUP BY ImageID) InnerStatement ` + matchingTagsFilter + ` ORDER BY ImageID DESC LIMIT ? OFFSET ?;`
	sqlCountQuery = sqlCountQuery + sqlWhereClause + `GROUP BY ImageID) InnerStatement ` + matchingTagsFilter

	//Now construct arguments list. Order must follow query order
	/*
		Inclusive Tags
		Exclusive Tags
		Inclusive Tag Count
		<However we pause here to run count query, as that one does not have limits>
		Max Amount of results to return
		Offset
	*/
	queryArray := []interface{}{}
	//Add inclusive tags to our queryArray
	for _, tag := range IncludeTags {
		queryArray = append(queryArray, tag)
	}
	//Add the exlusive tags
	for _, tag := range ExcludeTags {
		queryArray = append(queryArray, tag)
	}

	//Add values for metatags
	if len(MetaTags) > 0 {
		for _, tag := range MetaTags {
			queryArray = append(queryArray, tag.MetaValue)
		}
	}

	//Add inclusive tag count
	queryArray = append(queryArray, len(IncludeTags))

	//Run the count query (Count query does not use start/stride)
	err := DBConnection.DBHandle.QueryRow(sqlCountQuery, queryArray...).Scan(&MaxResults)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "SearchImages", "*", "ERROR", []string{"Erorr running search querry", sqlCountQuery, err.Error()})
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

//SearchImages performs a search for images (Returns a list of ImageInformations a result count and an error/nil)
func (DBConnection *MariaDBPlugin) searchImagesExclusive(ExcludeTags []uint64, MetaTags []interfaces.TagInformation, PageStart uint64, PageStride uint64) ([]interfaces.ImageInformation, uint64, error) {

	//Initialize output
	var ToReturn []interfaces.ImageInformation
	var MaxResults uint64

	//Construct SQL Query

	//This is the start of the query we want
	sqlQuery := `SELECT ID, Name, Location FROM Images `

	sqlCountQuery := `SELECT COUNT(*) FROM Images `

	//Now for the variable piece
	sqlWhereClause := ""
	if len(ExcludeTags) > 0 {
		sqlWhereClause = "WHERE ID NOT IN (SELECT DISTINCT ImageID FROM ImageTags WHERE TagID IN (?" + strings.Repeat(",?", len(ExcludeTags)-1) + ")) "
	}

	//And add any metatags
	if len(MetaTags) > 0 {
		firstPass := true
		for _, tag := range MetaTags {
			metaTagQuery := "AND "
			if firstPass && len(ExcludeTags) == 0 {
				metaTagQuery = "WHERE " //If we are processing a query only containing a metatag, then we need the where here, otherwise is added above
			}
			metaTagQuery = metaTagQuery + "Images." + tag.Name + " "
			comparator := tag.Comparator
			if tag.Exclude {
				comparator = getInvertedComparator(comparator)
			}
			if comparator == "" {
				return ToReturn, 0, errors.New("Failed to invert query to negate on " + tag.Name)
			}
			metaTagQuery = metaTagQuery + comparator + " ? "

			sqlWhereClause = sqlWhereClause + metaTagQuery
			firstPass = false
		}
	}

	//Finish off query
	sqlQuery = sqlQuery + sqlWhereClause + ` ORDER BY ID DESC LIMIT ? OFFSET ?;`
	sqlCountQuery = sqlCountQuery + sqlWhereClause

	//Now construct arguments list. Order must follow query order
	/*
		Exclusive Tags
		<However we pause here to run count query, as that one does not have limits>
		Max Amount of results to return
		Offset
	*/
	queryArray := []interface{}{}
	//Add the exlusive tags
	for _, tag := range ExcludeTags {
		queryArray = append(queryArray, tag)
	}

	//Add values for metatags
	if len(MetaTags) > 0 {
		for _, tag := range MetaTags {
			queryArray = append(queryArray, tag.MetaValue)
		}
	}

	//Run the count query (Count query does not use start/stride)
	err := DBConnection.DBHandle.QueryRow(sqlCountQuery, queryArray...).Scan(&MaxResults)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "SearchImages", "*", "ERROR", []string{"Erorr running search querry", sqlCountQuery, err.Error()})
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

//parseMetaTags fills in additional information for MetaTags and vets out non-MetaTags
func (DBConnection *MariaDBPlugin) parseMetaTags(MetaTags []interfaces.TagInformation) ([]interfaces.TagInformation, []error) {
	var ToReturn []interfaces.TagInformation
	var ErrorList []error
	for _, tag := range MetaTags {
		ToAdd := tag
		switch ToAdd.Name {
		//TODO: Add additional metatags here
		case "uploader":
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
		case "rating":
			ToAdd.Name = "Rating"
			ToAdd.Description = "The rating of the image"
			ToAdd.Exists = true
			ToAdd.Comparator = "=" //Clobber any other comparator requested. This one will only support equals
			//Since rating is a string, no futher processing needed!
		case "score":
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
		case "averagescore":
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
		case "totalscore":
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
		case "scorevoters":
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
		default:
			ErrorList = append(ErrorList, errors.New("MetaTag does not exist"))
		}
		ToReturn = append(ToReturn, ToAdd)
	}
	return ToReturn, ErrorList
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

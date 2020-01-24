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
var regexTagValue = regexp.MustCompile("[^a-zA-Z0-9_\\-\\.]")

func prepareTagName(Name string) string {
	//Lowercase Name -> Trimmed front and end of whitespace -> any inner whitespace reduced and underscored
	Name = regexWhiteSpace.ReplaceAllString(strings.TrimSpace(strings.ToLower(Name)), "_") //Replace all whitespace with _
	//Case of metatag
	if strings.Count(Name, ":") == 1 {
		//Assume a metatag
		NameValue := strings.Split(Name, ":")
		value, comparator := getTagComparator(NameValue[1]) //Strip comparator, so it does not get replaced by a _
		Name = regexTagName.ReplaceAllString(NameValue[0], "_") + ":" + comparator + regexTagValue.ReplaceAllString(value, "_")
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
		logging.LogInterface.WriteLog("MariaDBPlugin", "NewTag", "*", "ERROR", []string{"Failed to add tag dues to size of name/description", Name, Description})
		return 0, errors.New("name or description outside of right sizes")
	}

	resultInfo, err := DBConnection.DBHandle.Exec("INSERT INTO Tags (Name, Description, UploaderID) VALUES (?, ?, ?);", Name, Description, UploaderID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "NewTag", "*", "ERROR", []string{"Failed to add tag", err.Error()})
		return 0, err
	}
	id, _ := resultInfo.LastInsertId()
	logging.LogInterface.WriteLog("MariaDBPlugin", "NewTag", "*", "SUCCESS", []string{"Tag added", strconv.FormatUint(uint64(id), 10)})

	return uint64(id), err
}

//DeleteTag removes a tag
func (DBConnection *MariaDBPlugin) DeleteTag(TagID uint64) error {
	//Ensure not in use
	var useCount int
	if err := DBConnection.DBHandle.QueryRow("SELECT COUNT(*) AS UseCount FROM ImageTags WHERE TagID = ?", TagID).Scan(&useCount); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteTag", "*", "ERROR", []string{"Failed to get tag use information", err.Error()})
		return errors.New("failed to check tag to delete usage")
	}

	if useCount > 0 {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteTag", "*", "ERROR", []string{"Tag to delete is still in use", strconv.FormatUint(TagID, 10), "in use", strconv.Itoa(useCount)})
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
func (DBConnection *MariaDBPlugin) AddTag(TagIDs []uint64, ImageID uint64, LinkerID uint64) error {
	if len(TagIDs) == 0 {
		return errors.New("No tags provided")
	}
	//Validate tags, if some are alias, add alias instead, if a tag does not exist, error out
	var validatedTagIDs []uint64
	values := ""
	queryArray := []interface{}{}
	for i := 0; i < len(TagIDs); i++ {
		TagID := TagIDs[i]
		tagInfo, err := DBConnection.GetTag(TagID, false)
		if err != nil {
			return errors.New("Failed to validate tag " + strconv.FormatUint(TagID, 10))
		}
		values += " ("
		//If this is an alias, then add aliasedid instead
		if tagInfo.IsAlias {
			validatedTagIDs = append(validatedTagIDs, tagInfo.AliasedID)
			values += " ?,"
			queryArray = append(queryArray, tagInfo.AliasedID)
		} else {
			validatedTagIDs = append(validatedTagIDs, TagID)
			values += " ?,"
			queryArray = append(queryArray, TagID)
		}
		queryArray = append(queryArray, ImageID)
		queryArray = append(queryArray, LinkerID)
		values += " ?, ?),"
	}
	values = values[:len(values)-1] + " ON DUPLICATE KEY UPDATE LinkerID=?;" //Strip last comma, add end
	queryArray = append(queryArray, LinkerID)                                //For duplicate key update
	sqlQuery := "INSERT INTO ImageTags (TagID, ImageID, LinkerID) VALUES" + values
	if _, err := DBConnection.DBHandle.Exec(sqlQuery, queryArray...); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "AddTag", "*", "ERROR", []string{"Tags not added to image", strconv.FormatUint(ImageID, 10), sqlQuery, err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "AddTag", "*", "SUCCESS", []string{"Tags added", strconv.FormatUint(ImageID, 10)})
	return nil
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
func (DBConnection *MariaDBPlugin) GetTag(ID uint64, IncludeCount bool) (interfaces.TagInformation, error) {
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
	var TagCount uint64
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

	if IncludeCount {
		sqlQuery := "SELECT COUNT(*) as TagCount FROM ImageTags WHERE TagID=?"
		err := DBConnection.DBHandle.QueryRow(sqlQuery, ID).Scan(&TagCount)
		if err != nil {
			return interfaces.TagInformation{ID: ID, Exists: false}, err
		}
	}

	return interfaces.TagInformation{Name: Name, ID: ID, Description: SDescription, Exists: true, Exclude: false, UploaderID: UploaderID, UploadTime: UploadTime, AliasedID: AliasedID, IsAlias: IsAlias, UseCount: TagCount}, nil
}

//GetTagByName returns detailed information on one tag as queried by name
func (DBConnection *MariaDBPlugin) GetTagByName(Name string) (interfaces.TagInformation, error) {
	sqlQuery := "SELECT ID, Description, UploaderID, UploadTime, AliasedID, IsAlias FROM Tags WHERE Name=?"
	//Pass the sql query to DB
	//Placeholders for data returned by each row
	var Description sql.NullString
	var TagID uint64
	var UploaderID uint64
	var NUploadTime mysql.NullTime
	var UploadTime time.Time
	var AliasedID uint64
	var IsAlias bool
	err := DBConnection.DBHandle.QueryRow(sqlQuery, Name).Scan(&TagID, &Description, &UploaderID, &NUploadTime, &AliasedID, &IsAlias)
	if err != nil {
		return interfaces.TagInformation{Name: Name, Exists: false}, err
	}
	//If description is a valid non-null value, use it, else, use ""
	var SDescription string
	if Description.Valid {
		SDescription = Description.String
	}
	//De-nullify time if possible
	if NUploadTime.Valid {
		UploadTime = NUploadTime.Time
	}

	return interfaces.TagInformation{Name: Name, ID: TagID, Description: SDescription, Exists: true, Exclude: false, UploaderID: UploaderID, UploadTime: UploadTime, AliasedID: AliasedID, IsAlias: IsAlias}, nil
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
		tagInfo, err := DBConnection.GetTag(AliasedID, false)
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

//SearchTags returns a list of tags like the provided name, but only the ID, Name, Description, and IsAlias
func (DBConnection *MariaDBPlugin) SearchTags(name string, PageStart uint64, PageStride uint64, WildcardForwardOnly bool, SortByUsage bool) ([]interfaces.TagInformation, uint64, error) {
	var ToReturn []interfaces.TagInformation
	queryArray := []interface{}{}
	sqlQuery := "SELECT ID, Name, Description, IsAlias FROM Tags"
	sqlCountQuery := "SELECT Count(*) FROM Tags"

	if SortByUsage {
		sqlQuery = sqlQuery + " JOIN (SELECT TagID, COUNT(*) as 'Usage' FROM ImageTags GROUP BY TagID) Cnt ON Cnt.TagID = Tags.ID"
	}

	//Cleanup Query and alter if we were provided a name
	name = strings.TrimSpace(name)
	name = strings.Replace(name, "%", "", -1)
	if name != "" {
		if WildcardForwardOnly {
			name = name + "%"
		} else {
			name = "%" + name + "%"
		}
		sqlQuery = sqlQuery + " WHERE Name like ?"
		sqlCountQuery = sqlCountQuery + " WHERE Name like ?"
		queryArray = append(queryArray, name)
	}

	//Add the sorting to the query
	if SortByUsage {
		sqlQuery = sqlQuery + " ORDER BY Cnt.Usage DESC"
	} else {
		sqlQuery = sqlQuery + " ORDER BY Name"
	}

	//Add the limit at the end
	sqlQuery = sqlQuery + " LIMIT ? OFFSET ?"

	//Query Count
	//Run the count query (Count query does not use start/stride, so run this before we add those)
	var MaxResults uint64
	err := DBConnection.DBHandle.QueryRow(sqlCountQuery, queryArray...).Scan(&MaxResults)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "SearchTags", "*", "ERROR", []string{"Error running count query", sqlCountQuery, err.Error()})
		return nil, 0, err
	}
	//

	queryArray = append(queryArray, PageStride)
	queryArray = append(queryArray, PageStart)

	//Pass the sql query to DB
	rows, err := DBConnection.DBHandle.Query(sqlQuery, queryArray...)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		//If description is a valid non-null value, use it, else, use ""
		var SDescription string
		if Description.Valid {
			SDescription = Description.String
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.TagInformation{Name: Name, ID: ID, Description: SDescription, Exists: true, Exclude: false, IsAlias: IsAlias})
	}
	return ToReturn, MaxResults, nil
}

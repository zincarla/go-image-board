package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"strconv"
)

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

//ReplaceImageTags replaces all instances of ImageTags that have the specified tag with the new tag
func (DBConnection *MariaDBPlugin) ReplaceImageTags(OldTagID uint64, NewTagID uint64, LinkerID uint64) error {
	query := `UPDATE ImageTags
	SET TagID = ? , LinkerID=?
	WHERE TagID=? AND ImageID NOT IN
	(
		SELECT ImageID from ImageTags WHERE TagID=?
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

//inverts a tags comparator
func getInvertedComparator(comparator string) string {
	if comparator == "=" {
		return "!="
	}
	if comparator == ">" {
		return "<="
	}
	if comparator == "<" {
		return ">="
	}
	if comparator == ">=" {
		return "<"
	}
	if comparator == "<=" {
		return ">"
	}
	if comparator == "LIKE" {
		return "NOT LIKE"
	}
	return ""
}

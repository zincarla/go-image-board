package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

//--Collections

//NewCollection adds a collection with the provided information
func (DBConnection *MariaDBPlugin) NewCollection(Name string, Description string, UploaderID uint64) (uint64, error) {
	if len(Name) < 3 || len(Name) > 255 || len(Description) > 255 {
		logging.LogInterface.WriteLog("MariaDBPlugin", "NewCollection", "*", "ERROR", []string{"Failed to add collection due to name/description size", Name, Description})
		return 0, errors.New("name or description outside size range")
	}

	resultInfo, err := DBConnection.DBHandle.Exec("INSERT INTO Collections (Name, Description, UploaderID) VALUES (?, ?, ?);", Name, Description, UploaderID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "NewCollection", "*", "ERROR", []string{"Failed to add collection", err.Error()})
		return 0, err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "NewCollection", "*", "SUCCESS", []string{"Collection added"})
	id, _ := resultInfo.LastInsertId()
	return uint64(id), err
}

//DeleteCollection removes a collection
func (DBConnection *MariaDBPlugin) DeleteCollection(CollectionID uint64) error {
	//Ensure not in use
	_, err := DBConnection.DBHandle.Exec("DELETE FROM CollectionMembers WHERE CollectionID=?;", CollectionID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteCollection", "*", "ERROR", []string{"Colleciton to delete is still in use and members could not be removed", strconv.FormatUint(CollectionID, 10)})
		return errors.New("could not remove members from collection before deleting collection")
	}

	//Delete
	_, err = DBConnection.DBHandle.Exec("DELETE FROM Collections WHERE ID=?;", CollectionID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteCollection", "*", "ERROR", []string{"Failed to delete collection", err.Error(), strconv.FormatUint(CollectionID, 10)})
	} else {
		logging.LogInterface.WriteLog("MariaDBPlugin", "DeleteCollection", "*", "SUCCESS", []string{"Collection deleted", strconv.FormatUint(CollectionID, 10)})
	}
	return err
}

//UpdateCollection updates a pre-existing collection
func (DBConnection *MariaDBPlugin) UpdateCollection(CollectionID uint64, Name string, Description string) error {
	//Cleanup name
	if len(Name) < 3 || len(Name) > 255 || len(Description) > 255 {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollection", "*", "ERROR", []string{"Failed to update collection due to size of name/description", Name, Description})
		return errors.New("name or description outside of right sizes")
	}

	_, err := DBConnection.DBHandle.Exec("UPDATE Collections SET Name = ?, Description=? WHERE ID=?;", Name, Description, CollectionID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollection", "*", "ERROR", []string{"Failed to update collection", err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollection", "*", "SUCCESS", []string{"Collection updated"})
	return nil
}

//GetCollections returns a list of all collections, but only the ID, Name, Description
func (DBConnection *MariaDBPlugin) GetCollections(PageStart uint64, PageStride uint64) ([]interfaces.CollectionInformation, uint64, error) {
	var ToReturn []interfaces.CollectionInformation

	sqlQuery := `SELECT CL.ID, CL.Name, CL.Description, IFNULL(Location, "") AS Location, IFNULL(Counts.Members,0) as Members
	FROM Collections CL
	-- This part gets the number of members in a collection
	LEFT JOIN (
		SELECT CollectionID, Count(*) as Members
		FROM CollectionMembers
		GROUP BY CollectionID
	) Counts ON Counts.CollectionID = CL.ID
	-- This part gets a preview image location
	LEFT JOIN (
		SELECT Location, CollectionMembers.CollectionID as CollectionID, MIN(CollectionMembers.OrderWeight)
		FROM Images
		INNER JOIN CollectionMembers ON CollectionMembers.ImageID = Images.ID
		GROUP BY CollectionMembers.CollectionID
	) Preview ON Preview.CollectionID = CL.ID
	ORDER BY Name
	LIMIT ? OFFSET ?;`

	sqlCountQuery := `SELECT COUNT(*) AS Count FROM Collections`
	//Get Count query
	var MaxResults uint64
	//Run the count query (Count query does not use start/stride)
	err := DBConnection.DBHandle.QueryRow(sqlCountQuery).Scan(&MaxResults)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "GetCollections", "*", "ERROR", []string{"Error running count query", sqlCountQuery, err.Error()})
		return nil, 0, err
	}

	//Pass the sql query to DB
	rows, err := DBConnection.DBHandle.Query(sqlQuery, PageStride, PageStart)
	if err != nil {
		return nil, MaxResults, err
	}
	defer rows.Close()
	//Placeholders for data returned by each row
	var Description sql.NullString
	var ID uint64
	var Name string
	var Location string
	var Members uint64
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&ID, &Name, &Description, &Location, &Members)
		if err != nil {
			return nil, MaxResults, err
		}
		//If description is a valid non-null value, use it, else, use ""
		var SDescription string
		if Description.Valid {
			SDescription = Description.String
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.CollectionInformation{Name: Name, ID: ID, Description: SDescription, Location: Location, Members: Members})
	}
	return ToReturn, MaxResults, nil
}

//GetCollection returns detailed information on one collection
func (DBConnection *MariaDBPlugin) GetCollection(ID uint64) (interfaces.CollectionInformation, error) {
	sqlQuery := "SELECT Name, Description, UploaderID, UploadTime FROM Collections WHERE ID=?"
	//Pass the sql query to DB
	//Placeholders for data returned by each row
	var Description sql.NullString
	var Name string
	var UploaderID uint64
	var NUploadTime mysql.NullTime
	var UploadTime time.Time
	if err := DBConnection.DBHandle.QueryRow(sqlQuery, ID).Scan(&Name, &Description, &UploaderID, &NUploadTime); err != nil {
		return interfaces.CollectionInformation{}, err
	}

	var MemberCount uint64
	if err := DBConnection.DBHandle.QueryRow("SELECT COUNT(*) FROM CollectionMembers WHERE CollectionID=?", ID).Scan(&MemberCount); err != nil {
		return interfaces.CollectionInformation{}, err
	}

	//If description is a valid non-null value, use it, else, use ""
	var SDescription string
	if Description.Valid {
		SDescription = Description.String
	}

	if NUploadTime.Valid {
		UploadTime = NUploadTime.Time
	}

	return interfaces.CollectionInformation{Name: Name, ID: ID, Description: SDescription, UploaderID: UploaderID, UploadTime: UploadTime, Members: MemberCount}, nil
}

//GetCollectionByName returns detailed information on one collection
func (DBConnection *MariaDBPlugin) GetCollectionByName(Name string) (interfaces.CollectionInformation, error) {
	sqlQuery := "SELECT ID, Name, Description, UploaderID, UploadTime FROM Collections WHERE Name=?"
	//Pass the sql query to DB
	//Placeholders for data returned by each row
	var Description sql.NullString
	var CollectionID uint64
	var UploaderID uint64
	var NUploadTime mysql.NullTime
	var UploadTime time.Time
	if err := DBConnection.DBHandle.QueryRow(sqlQuery, Name).Scan(&CollectionID, &Name, &Description, &UploaderID, &NUploadTime); err != nil {
		return interfaces.CollectionInformation{}, err
	}

	var MemberCount uint64
	if err := DBConnection.DBHandle.QueryRow("SELECT COUNT(*) FROM CollectionMembers WHERE CollectionID=?", CollectionID).Scan(&MemberCount); err != nil {
		return interfaces.CollectionInformation{}, err
	}

	//If description is a valid non-null value, use it, else, use ""
	var SDescription string
	if Description.Valid {
		SDescription = Description.String
	}

	if NUploadTime.Valid {
		UploadTime = NUploadTime.Time
	}

	return interfaces.CollectionInformation{Name: Name, ID: CollectionID, Description: SDescription, UploaderID: UploaderID, UploadTime: UploadTime, Members: MemberCount}, nil
}

//--Collection Members

//AddCollectionMember adds an image to a collection
func (DBConnection *MariaDBPlugin) AddCollectionMember(CollectionID uint64, ImageIDs []uint64, LinkerID uint64) error {
	if len(ImageIDs) == 0 {
		return errors.New("ImageIDs required")
	}
	//Get last order
	lastOrder := uint64(0)
	memberCount := uint64(0)
	if err := DBConnection.DBHandle.QueryRow("SELECT IFNULL(MAX(OrderWeight),0) AS LastWeight, COUNT(*) AS MemberCount FROM CollectionMembers WHERE CollectionID = ?", CollectionID).Scan(&lastOrder, &memberCount); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "AddCollectionMember", "*", "ERROR", []string{"Could not get count of members in collection", strconv.FormatUint(CollectionID, 10)})
		return errors.New("could not get count of members in collection")
	}

	queryArray := []interface{}{}
	values := ""
	idString := ""
	//If we are not an empty collection, increment the number
	//Otherwise first image will have 0 as it's weight
	//We have to use a memberCount as a null OrderWeight is treated as 0, and a collection with one image would be 0
	if memberCount != 0 {
		lastOrder++
	}
	for i := 0; i < len(ImageIDs); i++ {
		values += " ( ?, ?, ?, ?),"
		queryArray = append(queryArray, CollectionID, ImageIDs[i], LinkerID, lastOrder)
		idString += strconv.FormatUint(ImageIDs[i], 10) + ", "
		lastOrder++
	}

	values = values[:len(values)-1] + ";" //Strip comma add semi

	//Add image
	sqlQuery := "INSERT INTO CollectionMembers (CollectionID, ImageID, LinkerID, OrderWeight) VALUES" + values
	if _, err := DBConnection.DBHandle.Exec(sqlQuery, queryArray...); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "AddCollectionMember", "*", "ERROR", []string{"Image not added to collection", strconv.FormatUint(CollectionID, 10), idString, err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "AddCollectionMember", "*", "SUCCESS", []string{"Image added to collection", strconv.FormatUint(CollectionID, 10), idString})
	return nil
}

//RemoveCollectionMember removes an image from collection
func (DBConnection *MariaDBPlugin) RemoveCollectionMember(CollectionID uint64, ImageID uint64) error {
	//Get Order
	var Order uint64
	if err := DBConnection.DBHandle.QueryRow("SELECT OrderWeight FROM CollectionMembers WHERE ImageID=? AND CollectionID=?", ImageID, CollectionID).Scan(&Order); err != nil {
		return err
	}

	var Members uint64
	if err := DBConnection.DBHandle.QueryRow("SELECT Count(*) FROM CollectionMembers WHERE CollectionID=?", CollectionID).Scan(&Members); err != nil {
		return err
	}

	//If last member of collection, just delete it instead
	if Members <= 1 {
		return DBConnection.DeleteCollection(CollectionID)
	}

	//Delete Image
	if _, err := DBConnection.DBHandle.Exec("DELETE FROM CollectionMembers WHERE CollectionID =? AND ImageID = ?;", CollectionID, ImageID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "RemoveCollectionMember", "*", "ERROR", []string{"Image not removed from collection", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "RemoveCollectionMember", "*", "SUCCESS", []string{"Image removed from collection", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10)})

	//Decrement Order
	if _, err := DBConnection.DBHandle.Exec("UPDATE CollectionMembers SET OrderWeight = OrderWeight - 1 WHERE OrderWeight > ? AND CollectionID=?;", Order, CollectionID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "RemoveCollectionMember", "*", "ERROR", []string{"Could not update Order after member removed from collection", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}

	return nil
}

//UpdateCollectionMember updates an image's properties in a collection
func (DBConnection *MariaDBPlugin) UpdateCollectionMember(CollectionID uint64, ImageID uint64, Order uint64) error {
	//Get Current Order
	var BeforeOrder uint64
	if err := DBConnection.DBHandle.QueryRow("SELECT OrderWeight FROM CollectionMembers WHERE ImageID=? AND CollectionID=?", ImageID, CollectionID).Scan(&BeforeOrder); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollectionMember", "*", "ERROR", []string{"Could not get previous order to update collectionmember", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}

	var MemberCount uint64
	if err := DBConnection.DBHandle.QueryRow("SELECT COUNT(*) FROM CollectionMembers WHERE CollectionID=?", CollectionID).Scan(&MemberCount); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollectionMember", "*", "ERROR", []string{"Could not validate order", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}

	//Ensure that we do not try and set this image to say, the 20th position when we have 3 images. Don't error, just silently set order to last image.
	if MemberCount <= Order {
		Order = MemberCount - 1 //-1 because we are ordering from 0. If we have 20 images, the last spot is actually 19
	}

	//Set order for image
	if _, err := DBConnection.DBHandle.Exec("UPDATE CollectionMembers SET OrderWeight = ? WHERE ImageID=? AND CollectionID=?;", Order, ImageID, CollectionID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollectionMember", "*", "ERROR", []string{"Could not set Order of member in collection", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}

	//Decrement Order
	if _, err := DBConnection.DBHandle.Exec("UPDATE CollectionMembers SET OrderWeight = OrderWeight - 1 WHERE OrderWeight >= ? AND CollectionID=? AND ImageID<>?;", BeforeOrder, CollectionID, ImageID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollectionMember", "*", "ERROR", []string{"Could not decrement Order of members in collection", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}

	//Increment Order
	if _, err := DBConnection.DBHandle.Exec("UPDATE CollectionMembers SET OrderWeight = OrderWeight + 1 WHERE OrderWeight >= ? AND CollectionID=? AND ImageID<>?;", Order, CollectionID, ImageID); err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateCollectionMember", "*", "ERROR", []string{"Could not increment Order of members in collection", strconv.FormatUint(CollectionID, 10), strconv.FormatUint(ImageID, 10), err.Error()})
		return err
	}

	return nil
}

//GetCollectionMembers gets a list of images in a collection (Returns a list of imageIDs, or error)
func (DBConnection *MariaDBPlugin) GetCollectionMembers(CollectionID uint64, PageStart uint64, PageStride uint64) ([]interfaces.ImageInformation, uint64, error) {
	//Attributes passed to SQL Query
	queryArray := []interface{}{}
	queryArray = append(queryArray, CollectionID)

	//Queries
	sqlQuery := `SELECT ImageID, Name, Location, OrderWeight
	FROM Images
	INNER JOIN CollectionMembers ON Images.ID=CollectionMembers.ImageID
	WHERE CollectionMembers.CollectionID=?
	ORDER BY CollectionMembers.OrderWeight`

	//If we limited the search
	if PageStride > 0 {
		//Add the limit and necessary parameters to array
		sqlQuery = sqlQuery + ` LIMIT ? OFFSET ?;`
		queryArray = append(queryArray, PageStride)
		queryArray = append(queryArray, PageStart)
	}

	sqlCountQuery := `SELECT COUNT(ImageID)
	FROM Images
	INNER JOIN CollectionMembers ON Images.ID=CollectionMembers.ImageID
	WHERE CollectionMembers.CollectionID=?;`

	//Init Output
	var ToReturn []interfaces.ImageInformation
	var MaxResults uint64

	//Run the count query (Count query does not use start/stride)
	err := DBConnection.DBHandle.QueryRow(sqlCountQuery, CollectionID).Scan(&MaxResults)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "GetCollectionMembers", "*", "ERROR", []string{"Error running count query", sqlCountQuery, err.Error()})
		return nil, 0, err
	}

	//Now for the real query
	rows, err := DBConnection.DBHandle.Query(sqlQuery, queryArray...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	//Placeholders for data returned by each row
	var ImageID uint64
	var Name string
	var Location string
	var Order uint64
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&ImageID, &Name, &Location, &Order)
		if err != nil {
			return nil, 0, err
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.ImageInformation{Name: Name, ID: ImageID, Location: Location, OrderInCollection: Order})
	}
	return ToReturn, MaxResults, nil
}

//GetCollectionsWithImage returns a slice of collections with a specific image
func (DBConnection *MariaDBPlugin) GetCollectionsWithImage(ImageID uint64) ([]interfaces.CollectionInformation, error) {
	var ToReturn []interfaces.CollectionInformation
	sqlQuery := `SELECT Collections.Name, Collections.Description, CollectionMembers.OrderWeight, Collections.ID, Counts.Members, IFNULL(BeforeMember.ImageID,0) as BeforeMember, IFNULL(AfterMember.ImageID,0) as AfterMember
	FROM CollectionMembers
	INNER JOIN Collections ON Collections.ID=CollectionMembers.CollectionID
	-- This part gets the number of members in a collection
	INNER JOIN (
		SELECT CollectionID, Count(*) as Members
		FROM CollectionMembers
		GROUP BY CollectionID
	) Counts ON Counts.CollectionID = Collections.ID
	-- This part gets the imageid for the previous image in collection or 0
	LEFT JOIN (
		SELECT IFNULL(ImageID,0) as ImageID, CollectionID
		FROM CollectionMembers CM
		WHERE OrderWeight < (SELECT OrderWeight FROM CollectionMembers WHERE ImageID = ? AND CollectionID = CM.CollectionID)
		ORDER BY OrderWeight DESC
		LIMIT 0,1
	) BeforeMember ON BeforeMember.CollectionID = Collections.ID
	-- This part gets the imageid for the next image in collection or 0
	LEFT JOIN (
		SELECT IFNULL(ImageID,0) as ImageID, CollectionID
		FROM CollectionMembers CM
		WHERE OrderWeight > (SELECT OrderWeight FROM CollectionMembers WHERE ImageID = ? AND CollectionID = CM.CollectionID)
		ORDER BY OrderWeight
		LIMIT 0,1
	) AfterMember ON AfterMember.CollectionID = Collections.ID
	WHERE CollectionMembers.ImageID=?`

	//First Query the main information
	rows, err := DBConnection.DBHandle.Query(sqlQuery, ImageID, ImageID, ImageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	//Placeholders for data returned by each row
	var Name string
	var Description string
	var Order uint64
	var CollectionID uint64
	var Members uint64
	var BeforeID uint64
	var AfterID uint64
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&Name, &Description, &Order, &CollectionID, &Members, &BeforeID, &AfterID)
		if err != nil {
			return nil, err
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.CollectionInformation{Name: Name, Description: Description, ID: CollectionID, OrderInCollection: Order, Members: Members, PreviousMemberID: BeforeID, NextMemberID: AfterID})
	}

	return ToReturn, nil
}

//GetCollectionTags returns a list of TagInformation for all tags that apply to the given collection
func (DBConnection *MariaDBPlugin) GetCollectionTags(CollectionID uint64) ([]interfaces.TagInformation, error) {
	var ToReturn []interfaces.TagInformation
	sqlQuery := "SELECT Tags.ID, Tags.Name, Tags.Description FROM CollectionTags INNER JOIN Tags ON Tags.ID = CollectionTags.TagID WHERE CollectionID=?"
	//Pass the sql query to DB
	rows, err := DBConnection.DBHandle.Query(sqlQuery, CollectionID)
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

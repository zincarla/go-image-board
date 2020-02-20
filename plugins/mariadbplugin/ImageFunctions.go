package mariadbplugin

import (
	"errors"
	"fmt"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"strconv"

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

//UpdateImage updates properties of an image
func (DBConnection *MariaDBPlugin) UpdateImage(ImageID uint64, ImageName interface{}, ImageDescription interface{}, OwnerID interface{}, Rating interface{}, Source interface{}, Location interface{}) error {
	if _, correctValue := OwnerID.(uint64); OwnerID != nil && correctValue == false {
		return errors.New("OwnerID, when provided, must be of uint64 type")
	}

	//See if image exists
	_, err := DBConnection.GetImage(ImageID)
	if err != nil {
		return err
	}

	queryArray := []interface{}{}
	sqlQuery := ""

	if ImageName != nil {
		queryArray = append(queryArray, fmt.Sprintf("%v", ImageName))
		if sqlQuery != "" {
			sqlQuery += ", "
		}
		sqlQuery += "Name = ? "
	}
	if ImageDescription != nil {
		queryArray = append(queryArray, fmt.Sprintf("%v", ImageDescription))
		if sqlQuery != "" {
			sqlQuery += ", "
		}
		sqlQuery += "Description = ? "
	}
	if unwrappedOwnerID, correctValue := OwnerID.(uint64); OwnerID != nil && correctValue {
		queryArray = append(queryArray, unwrappedOwnerID)
		if sqlQuery != "" {
			sqlQuery += ", "
		}
		sqlQuery += "UploaderID = ? "
	}
	if Rating != nil {
		queryArray = append(queryArray, fmt.Sprintf("%v", Rating))
		if sqlQuery != "" {
			sqlQuery += ", "
		}
		sqlQuery += "Rating = ? "
	}
	if Source != nil {
		queryArray = append(queryArray, fmt.Sprintf("%v", Source))
		if sqlQuery != "" {
			sqlQuery += ", "
		}
		sqlQuery += "Source = ? "
	}
	if Location != nil {
		queryArray = append(queryArray, fmt.Sprintf("%v", Location))
		if sqlQuery != "" {
			sqlQuery += ", "
		}
		sqlQuery += "Location = ? "
	}
	queryArray = append(queryArray, ImageID)
	if sqlQuery == "" {
		return nil //No change requested
	}
	sqlQuery = "UPDATE Images SET " + sqlQuery + "WHERE ID = ?"
	_, err = DBConnection.DBHandle.Exec(sqlQuery, queryArray...)
	return err
}

//GetImage returns information on a single image (Returns an ImageInformation, or error)
func (DBConnection *MariaDBPlugin) GetImage(ID uint64) (interfaces.ImageInformation, error) {
	ToReturn := interfaces.ImageInformation{ID: ID}
	var UploadTime mysql.NullTime
	err := DBConnection.DBHandle.QueryRow("Select Images.Name, IFNULL(Images.Description,'') AS Description, Images.Location, Images.UploaderID, Images.UploadTime, Images.Rating, Users.Name, Images.ScoreAverage, Images.ScoreTotal, Images.ScoreVoters, Images.Source FROM Images LEFT OUTER JOIN Users ON Images.UploaderID = Users.ID WHERE Images.ID=?", ID).Scan(&ToReturn.Name, &ToReturn.Description, &ToReturn.Location, &ToReturn.UploaderID, &UploadTime, &ToReturn.Rating, &ToReturn.UploaderName, &ToReturn.ScoreAverage, &ToReturn.ScoreTotal, &ToReturn.ScoreVoters, &ToReturn.Source)
	if err != nil {
		logging.LogInterface.WriteLog("ImageFunctions", "GetImage", "*", "ERROR", []string{"Failed to get image info from database", err.Error()})
		return ToReturn, err
	}
	if UploadTime.Valid {
		ToReturn.UploadTime = UploadTime.Time
	}
	return ToReturn, nil
}

//GetImageByFileName returns an ImageInformation object given a ImageName
func (DBConnection *MariaDBPlugin) GetImageByFileName(imageName string) (interfaces.ImageInformation, error) {
	ToReturn := interfaces.ImageInformation{Location: imageName}
	var UploadTime mysql.NullTime
	err := DBConnection.DBHandle.QueryRow("Select Images.Name, IFNULL(Images.Description,'') AS Description, Images.ID, Images.UploaderID, Images.UploadTime, Images.Rating, Users.Name, Images.ScoreAverage, Images.ScoreTotal, Images.ScoreVoters, Images.Source FROM Images LEFT OUTER JOIN Users ON Images.UploaderID = Users.ID WHERE Images.Location=?", imageName).Scan(&ToReturn.Name, &ToReturn.Description, &ToReturn.ID, &ToReturn.UploaderID, &UploadTime, &ToReturn.Rating, &ToReturn.UploaderName, &ToReturn.ScoreAverage, &ToReturn.ScoreTotal, &ToReturn.ScoreVoters, &ToReturn.Source)
	if err != nil {
		logging.LogInterface.WriteLog("ImageFunctions", "GetImageByFileName", "*", "ERROR", []string{"Failed to get image info from database", err.Error()})
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

package mariadbplugin

import (
	"database/sql"
	"go-image-board/logging"
)

//Score operations

//UpdateUserVoteScore Either creates or changes a user's vote on an image
func (DBConnection *MariaDBPlugin) UpdateUserVoteScore(UserID uint64, ImageID uint64, Score int64) error {
	//Check if user voted before
	sqlQuery := "SELECT COUNT(*) FROM ImageUserScores WHERE UserID=? AND ImageID=?;"
	count := 0
	err := DBConnection.DBHandle.QueryRow(sqlQuery, UserID, ImageID).Scan(&count)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateUserVoteScore", "*", "ERROR", []string{"Failed to verify score existance", err.Error()})
		return err
	}
	if count > 0 {
		//Update if so
		sqlQuery = "UPDATE ImageUserScores SET Score = ? WHERE UserID=? AND ImageID=?;"
	} else {
		//Create if not
		sqlQuery = "INSERT INTO ImageUserScores (Score, UserID, ImageID) VALUES (?, ?, ?);"
	}
	_, err = DBConnection.DBHandle.Exec(sqlQuery, Score, UserID, ImageID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateUserVoteScore", "*", "ERROR", []string{"Failed to update/add score", err.Error()})
		return err
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateUserVoteScore", "*", "SUCCESS", []string{"Score added/updated"})
	go DBConnection.UpdateScoreOnImage(ImageID)
	return nil
}

//UpdateScoreOnImage update ScoreTotal, ScoreAverage, and ScoreVoters on an image
func (DBConnection *MariaDBPlugin) UpdateScoreOnImage(ImageID uint64) error {
	sqlQuery := "SELECT COUNT(Score), SUM(Score), AVG(Score) FROM ImageUserScores WHERE ImageID=?;"
	var count, sum, average float64
	err := DBConnection.DBHandle.QueryRow(sqlQuery, ImageID).Scan(&count, &sum, &average)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateScoreOnImage", "*", "ERROR", []string{"Failed to pull score metrics", err.Error()})
		return err
	}
	sqlQuery = "UPDATE Images SET ScoreTotal = ?, ScoreAverage = ?, ScoreVoters = ? WHERE ID=?;"
	_, err = DBConnection.DBHandle.Exec(sqlQuery, sum, average, count, ImageID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateScoreOnImage", "*", "ERROR", []string{"Failed to update score for image", err.Error()})
		return err
	}
	return nil
}

//GetUserVoteScore Returns a user's vote on an image
func (DBConnection *MariaDBPlugin) GetUserVoteScore(UserID uint64, ImageID uint64) (int64, error) {
	//Check if user voted before
	sqlQuery := "SELECT Score FROM ImageUserScores WHERE UserID=? AND ImageID=?;"
	var score int64
	err := DBConnection.DBHandle.QueryRow(sqlQuery, UserID, ImageID).Scan(&score)
	if err != nil {
		if err != sql.ErrNoRows {
			logging.LogInterface.WriteLog("MariaDBPlugin", "UpdateUserVoteScore", "*", "ERROR", []string{"Failed to verify score existance", err.Error()})
			return 0, err
		}
	}
	return score, nil
}

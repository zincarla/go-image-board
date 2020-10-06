package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/logging"

	"golang.org/x/crypto/bcrypt"
)

//SetSecurityQuestions changes a user's security questions (nil if success)
func (DBConnection *MariaDBPlugin) SetSecurityQuestions(userName string, questionOne string, questionTwo string, questionThree string, answerOne []byte, answerTwo []byte, answerThree []byte, challengeAnswer []byte) error {
	answerOneHash, errA := getPasswordHash(answerOne)
	answerTwoHash, errB := getPasswordHash(answerTwo)
	answerThreeHash, errC := getPasswordHash(answerThree)

	if errA != nil || errB != nil || errC != nil {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/RevokeToken", userName, logging.ResultFailure, []string{"Failed to hash security question answers", userName})
		return errors.New("Failed to set answers")
	}

	//Grab pre-existing first quesion, if needed
	var secQuestionOne sql.NullString
	var secAnswerOne sql.NullString
	err := DBConnection.DBHandle.QueryRow("SELECT SecQuestionOne, SecAnswerOne FROM Users WHERE Name = ?", userName).Scan(&secQuestionOne, &secAnswerOne)
	//If question one is set
	if err != nil {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/SetSecurityQuestions", userName, logging.ResultFailure, []string{"Security questions failed to update. Challenge could not be loaded SQL Error.", userName, err.Error()})
		return errors.New("sql error occured attempt to load old question")
	}
	if secQuestionOne.Valid && secQuestionOne.String != "" {
		//Challenge needed/Require that the user entered in the answer to q1
		if bcrypt.CompareHashAndPassword([]byte(secAnswerOne.String), challengeAnswer) != nil {
			//Challenge failed/If we fail, log it, and quit without setting questions
			logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/SetSecurityQuestions", userName, logging.ResultFailure, []string{"Security questions failed to update. Challenge answer incorrect or SQL error.", userName})
			return errors.New("provided answer did not pass challenge")
		}
	}

	_, err = DBConnection.DBHandle.Exec("UPDATE Users SET SecQuestionOne=?, SecQuestionTwo=?, SecQuestionThree=?, SecAnswerOne=?, SecAnswerTwo=?, SecAnswerThree=? WHERE Name = ? AND Disabled = FALSE", questionOne, questionTwo, questionThree, string(answerOneHash), string(answerTwoHash), string(answerThreeHash), userName)
	if err == nil {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/SetSecurityQuestions", userName, logging.ResultSuccess, []string{"Security questions updated!", userName})
	} else {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/SetSecurityQuestions", userName, logging.ResultFailure, []string{"Security questions failed to update", userName, err.Error()})
	}
	return err
}

//ValidateSecurityQuestions Validates answers against a user's security questions (nil on success)
func (DBConnection *MariaDBPlugin) ValidateSecurityQuestions(userName string, answerOne []byte, answerTwo []byte, answerThree []byte) error {
	//Ensure answers have values
	if answerOne == nil || answerTwo == nil || answerThree == nil {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/ValidateSecurityQuestions", userName, logging.ResultFailure, []string{"No answers?", userName})
		return errors.New("Security Question validation failed, provide answers")
	}

	//Ensure Questions Exist
	secQuestionOne, secQuestionTwo, secQuestionThree, err := DBConnection.GetSecurityQuestions(userName)
	if err != nil || secQuestionOne == "" || secQuestionTwo == "" || secQuestionThree == "" {

		if err != nil {
			logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/ValidateSecurityQuestions", userName, logging.ResultFailure, []string{"User does not exist?", err.Error(), userName})
			return err
		}
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/ValidateSecurityQuestions", userName, logging.ResultFailure, []string{"Questions do not exist for user", userName})
		return errors.New("Questions do not exist for user")
	}

	var secAnswerOne sql.NullString
	var secAnswerTwo sql.NullString
	var secAnswerThree sql.NullString

	row := DBConnection.DBHandle.QueryRow("SELECT SecAnswerOne, SecAnswerTwo, SecAnswerThree FROM Users WHERE Name = ?", userName)
	err = row.Scan(&secAnswerOne, &secAnswerTwo, &secAnswerThree)
	if err != nil {
		return err
	}

	if secAnswerOne.Valid && secAnswerTwo.Valid && secAnswerThree.Valid != true {
		return errors.New("Account does not have answers to one or more questions")
	}

	if bcrypt.CompareHashAndPassword([]byte(secAnswerOne.String), answerOne) != nil {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/ValidateSecurityQuestions", userName, logging.ResultFailure, []string{"Answer 1 incorrect", userName})
		return errors.New("Security Question validation failed")
	}

	if bcrypt.CompareHashAndPassword([]byte(secAnswerTwo.String), answerTwo) != nil {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/ValidateSecurityQuestions", userName, logging.ResultFailure, []string{"Answer 2 incorrect", userName})
		return errors.New("Security Question validation failed")
	}

	if bcrypt.CompareHashAndPassword([]byte(secAnswerThree.String), answerThree) != nil {
		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/ValidateSecurityQuestions", userName, logging.ResultFailure, []string{"Answer 3 incorrect", userName})
		return errors.New("Security Question validation failed")
	}

	return nil
}

//GetSecurityQuestions returns the three questions, first, second, third, and an error if an issue occured
func (DBConnection *MariaDBPlugin) GetSecurityQuestions(userName string) (string, string, string, error) {
	var secQuestionOne sql.NullString
	var secQuestionTwo sql.NullString
	var secQuestionThree sql.NullString
	row := DBConnection.DBHandle.QueryRow("SELECT SecQuestionOne, SecQuestionTwo, SecQuestionThree FROM Users WHERE Name = ?", userName)
	err := row.Scan(&secQuestionOne, &secQuestionTwo, &secQuestionThree)
	if err != nil {
		return "", "", "", err
	}
	if secQuestionOne.Valid && secQuestionTwo.Valid && secQuestionThree.Valid {
		return secQuestionOne.String, secQuestionTwo.String, secQuestionThree.String, nil
	}
	return "", "", "", errors.New("one or more questions nil")
}

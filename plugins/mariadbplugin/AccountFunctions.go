package mariadbplugin

import (
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

//CreateUser is used to create and add a user to the AuthN database (return nil on success)
func (DBConnection *MariaDBPlugin) CreateUser(userName string, password []byte, email string, permissions uint64) error {
	//Validate User does not exist
	var userCount int
	row := DBConnection.DBHandle.QueryRow("SELECT COUNT(*) AS UserCount FROM Users WHERE Name = ? OR EMail = ?", userName, email)
	if err := row.Scan(&userCount); err != nil {
		return err
	}
	if err := DBConnection.ValidatePasswordStrength(string(password)); err != nil {
		return err
	}
	if userCount != 0 {
		return errors.New("Username or email already taken")
	}
	hash, err := getPasswordHash(password)
	if err != nil {
		return errors.New("Error with user password")
	}
	_, err = DBConnection.DBHandle.Exec("INSERT INTO Users (Name, EMail, PasswordHash, Permissions) VALUES (?, ?, ?, ?);", userName, email, string(hash), permissions)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "CreateUser", "*", "ERROR", []string{"Failed to create new user", err.Error()})
	}
	logging.LogInterface.WriteLog("MariaDBPlugin", "CreateUser", userName, "SUCCESS", []string{"New user added to database", userName})
	return err
}

//ValidateUser Validate a user's password (return nil if valid)
func (DBConnection *MariaDBPlugin) ValidateUser(userName string, password []byte) error {
	var userPassword string
	var userDisabled bool
	row := DBConnection.DBHandle.QueryRow("SELECT PasswordHash, Disabled FROM Users WHERE Name = ?", userName)
	err := row.Scan(&userPassword, &userDisabled)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "ValidateUser", userName, "ERROR", []string{"Username and Password not correct", userName, err.Error()})
		return err
	}
	if userDisabled {
		return errors.New("Account disabled")
	}
	result := bcrypt.CompareHashAndPassword([]byte(userPassword), password)
	if result == nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "ValidateUser", userName, "SUCCESS", []string{"Username and Password Correct", userName})
	} else {
		logging.LogInterface.WriteLog("MariaDBPlugin", "ValidateUser", userName, "ERROR", []string{"Password incorrect", userName})
	}
	return result
}

//GetUserID returns a user's DBID for association with other db elements
func (DBConnection *MariaDBPlugin) GetUserID(userName string) (uint64, error) {
	var userID uint64
	row := DBConnection.DBHandle.QueryRow("SELECT ID FROM Users WHERE Name = ?", userName)
	err := row.Scan(&userID)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "GetUserID", userName, "ERROR", []string{"Username does not exist", userName})
		return 0, err
	}
	return userID, nil
}

//GetUserPermissionSet returns a UserPermission object representing a user's intended access
func (DBConnection *MariaDBPlugin) GetUserPermissionSet(userName string) (interfaces.UserPermission, error) {
	var userPermission uint64
	row := DBConnection.DBHandle.QueryRow("SELECT Permissions FROM Users WHERE Name = ?", userName)
	err := row.Scan(&userPermission)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "GetUserID", userName, "ERROR", []string{"Username does not exist", userName})
		return 0, err
	}
	return interfaces.UserPermission(userPermission), nil
}

//SetUserPermissionSet sets a user's permission in the database
func (DBConnection *MariaDBPlugin) SetUserPermissionSet(userID uint64, permissions uint64) error {
	_, err := DBConnection.DBHandle.Exec("UPDATE Users SET Permissions=? WHERE ID=?", permissions, userID)
	return err
}

//SetUserDisableState disables or enables a user account
func (DBConnection *MariaDBPlugin) SetUserDisableState(userID uint64, isDisabled bool) error {
	_, err := DBConnection.DBHandle.Exec("UPDATE Users SET Disabled=? WHERE ID=?", isDisabled, userID)
	return err
}

//SetUserPassword Update a user's password, validation of user provided by either old password, or security answers. (nil on success)
func (DBConnection *MariaDBPlugin) SetUserPassword(userName string, password []byte, newPassword []byte, answerOne []byte, answerTwo []byte, answerThree []byte) error {
	//Validate authentication method
	if password == nil {
		if err := DBConnection.ValidateSecurityQuestions(userName, answerOne, answerTwo, answerThree); err != nil {
			//Need to use security question method
			return err
		}
	} else if err := DBConnection.ValidateUser(userName, password); err != nil {
		//Otherwise, utilize classic password
		return err
	}

	//At this point, we have passed the authentication (either security question or old password) now we need to change the password
	//Validate password meets strength requirements
	if err := DBConnection.ValidatePasswordStrength(string(newPassword)); err != nil {
		return err
	}
	//Hash it
	newPasswordHash, err := getPasswordHash(newPassword)
	if err != nil {
		return err
	}

	_, err = DBConnection.DBHandle.Exec("UPDATE Users SET PasswordHash=? WHERE Name = ?", string(newPasswordHash), userName)
	return err
}

//RemoveUser Removes a user from the database (nil on success)
func (DBConnection *MariaDBPlugin) RemoveUser(userName string) error {
	_, err := DBConnection.DBHandle.Exec("DELETE FROM Users WHERE Name = ?", userName)
	if err == nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "RemoveUser", userName, "SUCCESS", []string{"User removed", userName})
	} else {
		logging.LogInterface.WriteLog("MariaDBPlugin", "RemoveUser", userName, "ERROR", []string{"User not removed", userName, err.Error()})
	}
	return err
}

//ValidatePasswordStrength validates whether a user's password passes complexity requirements
func (DBConnection *MariaDBPlugin) ValidatePasswordStrength(password string) error {
	match, err := regexp.MatchString("^[a-zA-Z\\d\\!\\@\\#\\$\\%\\^\\&\\*\\(\\)\\-\\_\\=\\+]{3,60}$", string(password))
	if match == false {
		return errors.New("Password using invalid characters. alphanumeric and !@#$%^&*()_+=- between 3 and 60 characters")
	}
	return err
}

//Support Functions
//getPasswordHash Gets bcrypt hash from password
func getPasswordHash(password []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword(password, 14)
}

//ValidateProposedUsername returns whether a username is in a valid format
func (DBConnection *MariaDBPlugin) ValidateProposedUsername(UserName string) error {
	match, err := regexp.MatchString("^[a-zA-Z\\d]{3,20}$", UserName)
	if match == false {
		return errors.New("username using invalid characters. alphanumeric only between 3 and 20 characters")
	}
	if err != nil {
		return err
	}
	return nil
}

package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/config"
	"go-image-board/logging"
	"strconv"
	//I mean, where else would this go?
	_ "github.com/go-sql-driver/mysql"
)

//TODO: Increment this whenever we alter the DB Schema, ensure you attempt to add update code below
var currentDBVersion int64 = 2

//TODO: Increment this when we alter the db schema and don't add update code to compensate
var minSupportedDBVersion int64 = 0

//MariaDBPlugin acts as plugin between gib and a Maria/MySQL DB
type MariaDBPlugin struct {
	DBHandle *sql.DB
}

//InitDatabase connects to a database, and if needed, creates and or updates tables
func (DBConnection *MariaDBPlugin) InitDatabase() error {
	var err error
	//https://github.com/go-sql-driver/mysql/#dsn-data-source-name
	DBConnection.DBHandle, err = sql.Open("mysql", config.Configuration.DBUser+":"+config.Configuration.DBPassword+"@tcp("+config.Configuration.DBHost+":"+config.Configuration.DBPort+")/"+config.Configuration.DBName)
	if err == nil {
		err = DBConnection.DBHandle.Ping() //Ping actually validates we can query databse
		if err == nil {
			version, err := DBConnection.getDatabaseVersion()
			if err == nil {
				logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "INFO", []string{"DBVersion is " + strconv.FormatInt(version, 10)})
				if version < minSupportedDBVersion {
					return errors.New("database version is not supported and no update code was found to bring database up to current version")
				} else if version < currentDBVersion {
					version, err = DBConnection.upgradeDatabase(version)
					if err != nil {
						return err
					}
				}
			} else {
				logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "WARNING", []string{"Failed to get database verion, assuming not installed. Will attempt to perform install.", err.Error()})
				//Assume no database installed. Perform fresh install
				return DBConnection.performFreshDBInstall()
			}
		}
	}

	return err
}

//GetPluginInformation Return plugin info as string
func (DBConnection *MariaDBPlugin) GetPluginInformation() string {
	return "MariaDBPlugin 0.0.0.3"
}

func (DBConnection *MariaDBPlugin) getDatabaseVersion() (int64, error) {
	var version int64
	row := DBConnection.DBHandle.QueryRow("SELECT version FROM DBVersion")
	err := row.Scan(&version)
	return version, err
}

//performFreshDBInstall Installs the necessary tables for the application. This assumes that the database has not been created before
func (DBConnection *MariaDBPlugin) performFreshDBInstall() error {
	//DBVersion
	_, err := DBConnection.DBHandle.Exec("CREATE TABLE DBVersion (version BIGINT UNSIGNED NOT NULL);")
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("INSERT INTO DBVersion (version) VALUES (?);", currentDBVersion)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	//Images and tags
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE Tags (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, Name VARCHAR(255) NOT NULL UNIQUE, Description VARCHAR(255), UploaderID BIGINT UNSIGNED NOT NULL, UploadTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, AliasedID BIGINT UNSIGNED NOT NULL DEFAULT 0, IsAlias BOOL NOT NULL DEFAULT FALSE);")
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE ImageTags (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, ImageID BIGINT UNSIGNED NOT NULL, TagID BIGINT UNSIGNED NOT NULL, LinkerID BIGINT UNSIGNED NOT NULL, LinkTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageTagPair (TagID,ImageID));")
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE Images (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UploaderID BIGINT UNSIGNED NOT NULL, Name VARCHAR(255) NOT NULL, Rating VARCHAR(255) DEFAULT 'unrated', ScoreTotal BIGINT NOT NULL DEFAULT 0, ScoreAverage BIGINT NOT NULL DEFAULT 0, ScoreVoters BIGINT NOT NULL DEFAULT 0, Location VARCHAR(255) UNIQUE NOT NULL, UploadTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL);")
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE ImageUserScores (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UserID BIGINT UNSIGNED NOT NULL, ImageID BIGINT UNSIGNED NOT NULL, Score BIGINT NOT NULL, CreationTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageUserPair (UserID,ImageID));")
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	//Users
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE Users (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, Name VARCHAR(40) NOT NULL UNIQUE, EMail VARCHAR(255) NOT NULL UNIQUE, PasswordHash VARCHAR(255) NOT NULL, TokenID VARCHAR(255), IP VARCHAR(50), SecQuestionOne VARCHAR(50), SecQuestionTwo VARCHAR(50), SecQuestionThree VARCHAR(50), SecAnswerOne VARCHAR(255), SecAnswerTwo VARCHAR(255), SecAnswerThree VARCHAR(255), CreationTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, Disabled BOOL NOT NULL DEFAULT FALSE, Permissions BIGINT UNSIGNED NOT NULL DEFAULT 0);")
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	//Reserve system for auditing
	_, err = DBConnection.DBHandle.Exec("INSERT INTO Users (ID, Name, EMail, PasswordHash, Disabled) VALUES (?, ?, ?, ?, ?);", 0, "SYSTEM", "", "", true)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	//Auditing
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE AuditLogs (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UserID BIGINT UNSIGNED NOT NULL, Type VARCHAR(40), Info VARCHAR(255));")
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
		return err
	}
	return nil
}

//TODO: Add update code here
func (DBConnection *MariaDBPlugin) upgradeDatabase(version int64) (int64, error) {
	//Update version 0 -> 1
	if version == 0 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD COLUMN (Rating VARCHAR(255) DEFAULT 'unrated');")
		if err != nil {
			logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "ERROR", []string{"Failed to update database columns", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 1;")
		if err != nil {
			logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "ERROR", []string{"Failed to update database version", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE Images SET Rating = 'unrated';")
		if err != nil {
			logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "WARN", []string{"Failed to update rating on images after update", err.Error()})
			//Update was technically successfull, not returning for this error
		}
		version = 1
		logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "INFO", []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 1->2
	if version == 1 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD COLUMN (ScoreTotal BIGINT NOT NULL DEFAULT 0, ScoreAverage BIGINT NOT NULL DEFAULT 0, ScoreVoters BIGINT NOT NULL DEFAULT 0);")
		if err != nil {
			logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "ERROR", []string{"Failed to update database columns", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("CREATE TABLE ImageUserScores (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UserID BIGINT UNSIGNED NOT NULL, ImageID BIGINT UNSIGNED NOT NULL, Score BIGINT NOT NULL, CreationTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageUserPair (UserID,ImageID));")
		if err != nil {
			logging.LogInterface.WriteLog("MariaDBPlugin", "performFreshDBInstall", "*", "ERROR", []string{"Failed to install database", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 2;")
		if err != nil {
			logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "ERROR", []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 2
		logging.LogInterface.WriteLog("MariaDBPlugin", "InitDatabase", "*", "INFO", []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	return version, nil
}

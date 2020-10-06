package mariadbplugin

import (
	"database/sql"
	"errors"
	"go-image-board/config"
	"go-image-board/logging"
	"strconv"

	"math/rand"
	"time"

	//I mean, where else would this go?
	_ "github.com/go-sql-driver/mysql"
)

//TODO: Increment this whenever we alter the DB Schema, ensure you attempt to add update code below
var currentDBVersion int64 = 13

//TODO: Increment this when we alter the db schema and don't add update code to compensate
var minSupportedDBVersion int64 // 0 by default

//MariaDBPlugin acts as plugin between gib and a Maria/MySQL DB
type MariaDBPlugin struct {
	DBHandle *sql.DB
}

//InitDatabase connects to a database, and if needed, creates and or updates tables
func (DBConnection *MariaDBPlugin) InitDatabase() error {
	rand.Seed(time.Now().UnixNano())
	var err error
	//https://github.com/go-sql-driver/mysql/#dsn-data-source-name
	DBConnection.DBHandle, err = sql.Open("mysql", config.Configuration.DBUser+":"+config.Configuration.DBPassword+"@tcp("+config.Configuration.DBHost+":"+config.Configuration.DBPort+")/"+config.Configuration.DBName)
	if err == nil {
		err = DBConnection.DBHandle.Ping() //Ping actually validates we can query database
		if err == nil {
			version, err := DBConnection.getDatabaseVersion()
			if err == nil {
				logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"DBVersion is " + strconv.FormatInt(version, 10)})
				if version < minSupportedDBVersion {
					return errors.New("database version is not supported and no update code was found to bring database up to current version")
				} else if version < currentDBVersion {
					version, err = DBConnection.upgradeDatabase(version)
					if err != nil {
						return err
					}
				}
				//Validate Events
				var EventsEnabled string
				row := DBConnection.DBHandle.QueryRow("SELECT @@global.event_scheduler")
				err := row.Scan(&EventsEnabled)
				if err != nil {
					logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to get event scheduler setting", err.Error()})
				} else {
					if EventsEnabled != "ON" {
						logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Event scheduler is set to", EventsEnabled, "this may prevent automatic maitenance tasks from running"})
					}
				}
			} else {
				logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to get database version, assuming not installed. Will attempt to perform install.", err.Error()})
				//Assume no database installed. Perform fresh install
				return DBConnection.performFreshDBInstall()
			}
		}
	}

	return err
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
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("INSERT INTO DBVersion (version) VALUES (?);", currentDBVersion)
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	//Images and tags
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE Tags (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, Name VARCHAR(255) NOT NULL UNIQUE, Description VARCHAR(255), UploaderID BIGINT UNSIGNED NOT NULL, UploadTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, AliasedID BIGINT UNSIGNED NOT NULL DEFAULT 0, IsAlias BOOL NOT NULL DEFAULT FALSE);")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE ImageTags (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, ImageID BIGINT UNSIGNED NOT NULL, TagID BIGINT UNSIGNED NOT NULL, LinkerID BIGINT UNSIGNED NOT NULL, LinkTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageTagPair (TagID,ImageID), INDEX(ImageID), INDEX(LinkerID), CONSTRAINT fk_ImageTagsImageID FOREIGN KEY (ImageID) REFERENCES Images(ID), CONSTRAINT fk_ImageTagsTagID FOREIGN KEY (TagID) REFERENCES Tags(ID));")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE ImagedHashes (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, ImageID BIGINT UNSIGNED NOT NULL, vHash BIGINT UNSIGNED NOT NULL, hHash BIGINT UNSIGNED NOT NULL, UNIQUE INDEX(ImageID), INDEX(vHash), INDEX(hHash), CONSTRAINT fk_ImagedHashesImageID FOREIGN KEY (ImageID) REFERENCES Images(ID));")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE Images (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UploaderID BIGINT UNSIGNED NOT NULL, Name VARCHAR(255) NOT NULL, Rating VARCHAR(255) DEFAULT 'unrated', ScoreTotal BIGINT NOT NULL DEFAULT 0, ScoreAverage BIGINT NOT NULL DEFAULT 0, ScoreVoters BIGINT NOT NULL DEFAULT 0, Location VARCHAR(255) UNIQUE NOT NULL, Source VARCHAR(2000) NOT NULL DEFAULT '', UploadTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, Description TEXT NOT NULL DEFAULT '', INDEX(UploaderID), INDEX(Rating), INDEX(UploadTime), INDEX(ScoreAverage));")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE ImageUserScores (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UserID BIGINT UNSIGNED NOT NULL, ImageID BIGINT UNSIGNED NOT NULL, Score BIGINT NOT NULL, CreationTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageUserPair (UserID,ImageID));")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	//Users
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE Users (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, Name VARCHAR(40) NOT NULL UNIQUE, EMail VARCHAR(255) NOT NULL UNIQUE, PasswordHash VARCHAR(255) NOT NULL, TokenID VARCHAR(255), IP VARCHAR(50), SecQuestionOne VARCHAR(50), SecQuestionTwo VARCHAR(50), SecQuestionThree VARCHAR(50), SecAnswerOne VARCHAR(255), SecAnswerTwo VARCHAR(255), SecAnswerThree VARCHAR(255), CreationTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, Disabled BOOL NOT NULL DEFAULT FALSE, Permissions BIGINT UNSIGNED NOT NULL DEFAULT 0, SearchFilter VARCHAR(255) NOT NULL DEFAULT '');")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	//Reserve system for auditing
	_, err = DBConnection.DBHandle.Exec("INSERT INTO Users (ID, Name, EMail, PasswordHash, Disabled) VALUES (?, ?, ?, ?, ?);", 0, "SYSTEM", "", "", true)
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	//Auditing
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE AuditLogs (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UserID BIGINT UNSIGNED NOT NULL, Type VARCHAR(40), Info VARCHAR(10240) NOT NULL DEFAULT '', LogTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL);")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	//Collections
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE Collections (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, Name VARCHAR(255) NOT NULL UNIQUE, Description VARCHAR(255), UploaderID BIGINT UNSIGNED NOT NULL, UploadTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL);")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE CollectionMembers (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, ImageID BIGINT UNSIGNED NOT NULL, CollectionID BIGINT UNSIGNED NOT NULL, LinkerID BIGINT UNSIGNED NOT NULL, LinkTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageCollectionPair (CollectionID,ImageID), OrderWeight BIGINT UNSIGNED NOT NULL, CONSTRAINT fk_CollectionMembersImageID FOREIGN KEY (ImageID) REFERENCES Images(ID), CONSTRAINT fk_CollectionMembersCollectionID FOREIGN KEY (CollectionID) REFERENCES Collections(ID));")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	_, err = DBConnection.DBHandle.Exec("CREATE TABLE CollectionTags (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, CollectionID BIGINT UNSIGNED NOT NULL, TagID BIGINT UNSIGNED NOT NULL, LinkerID BIGINT UNSIGNED NOT NULL, LinkTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX CollectionTagPair (TagID,CollectionID), CONSTRAINT fk_CollectionTagsCollectionID FOREIGN KEY (CollectionID) REFERENCES Collections(ID), CONSTRAINT fk_CollectionTagsTagID FOREIGN KEY (TagID) REFERENCES Tags(ID));")
	if err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}
	//Stored Procedures, Triggers, Events
	sqlQuery := `CREATE PROCEDURE LinkCollTags(IN collID BIGINT UNSIGNED)
	BEGIN
	-- Insert missing tags
	INSERT INTO CollectionTags (TagID, CollectionID, LinkerID)
	SELECT DISTINCT ImageTags.TagID, collID, ImageTags.LinkerID
	FROM ImageTags
	INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
	LEFT JOIN CollectionTags on CollectionTags.CollectionID = CollectionMembers.CollectionID AND CollectionTags.TagID = ImageTags.TagID
	WHERE CollectionMembers.CollectionID = collID AND CollectionTags.CollectionID IS NULL;
	-- Remove extra tags
	DELETE FROM CollectionTags
	WHERE TagID NOT IN ( SELECT TagID 
							FROM ImageTags 
							INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
							WHERE CollectionMembers.CollectionID = collID
						)
	AND CollectionID=collID;
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE PROCEDURE AddMissingCollectionImageTags(IN imgID BIGINT UNSIGNED)
	BEGIN
		-- Insert missing tags
		INSERT INTO CollectionTags(TagID, CollectionID, LinkerID)
	SELECT DISTINCT
		ImageTags.TagID,
		CollectionMembers.CollectionID,
		ImageTags.LinkerID
	FROM
		ImageTags
	INNER JOIN CollectionMembers ON CollectionMembers.ImageID = ImageTags.ImageID
	LEFT JOIN CollectionTags ON CollectionTags.CollectionID = CollectionMembers.CollectionID AND CollectionTags.TagID = ImageTags.TagID
	WHERE
		CollectionTags.CollectionID IS NULL AND ImageTags.ImageID = imgID;
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE PROCEDURE RemSurplusCollectionImageTags(IN collID BIGINT UNSIGNED)
	BEGIN
		-- Remove extra tags
		DELETE FROM CollectionTags
		WHERE TagID NOT IN ( SELECT TagID 
								FROM ImageTags 
								INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
								WHERE CollectionMembers.CollectionID = collID
							)
		AND CollectionID=collID;
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE EVENT auditCleanup 
	ON SCHEDULE EVERY 1 DAY 
	DO 
	DELETE FROM AuditLogs WHERE LogTime < DATE_SUB(current_timestamp(), INTERVAL 30 DAY);`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE TRIGGER onCollectionDelete BEFORE DELETE ON Collections
	FOR EACH ROW BEGIN
		DELETE FROM CollectionMembers WHERE CollectionID=OLD.ID;
		DELETE FROM CollectionTags WHERE CollectionID=OLD.ID;
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE TRIGGER onCollectionMemberAdd AFTER INSERT ON CollectionMembers
	FOR EACH ROW BEGIN
		CALL AddMissingCollectionImageTags(NEW.ImageID);
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE TRIGGER onCollectionMemberDelete AFTER DELETE ON CollectionMembers
	FOR EACH ROW BEGIN
		CALL RemSurplusCollectionImageTags(OLD.CollectionID);
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE TRIGGER onImageTagDelete AFTER DELETE ON ImageTags
	FOR EACH ROW BEGIN
		DECLARE collID BIGINT UNSIGNED;
		DECLARE cursorDone BOOL DEFAULT FALSE;
		DECLARE collCursor CURSOR FOR SELECT CollectionID FROM CollectionMembers WHERE ImageID = OLD.ImageID;
		DECLARE CONTINUE HANDLER FOR NOT FOUND SET cursorDone = TRUE;
		OPEN collCursor;
		collLoop: LOOP
			FETCH collCursor INTO collID;
			IF cursorDone THEN
				LEAVE collLoop;
			END IF;
			CALL RemSurplusCollectionImageTags(collID);
		END LOOP;
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE TRIGGER onImageDelete BEFORE DELETE ON Images
	FOR EACH ROW BEGIN
		DELETE FROM ImageTags WHERE ImageID=OLD.ID;
		DELETE FROM ImageUserScores WHERE ImageID=OLD.ID;
		DELETE FROM CollectionMembers WHERE ImageID=OLD.ID;
		DELETE FROM ImagedHashes WHERE ImageID=OLD.ID;
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE TRIGGER onImageTagInsert AFTER INSERT ON ImageTags
	FOR EACH ROW BEGIN
		CALL AddMissingCollectionImageTags(NEW.ImageID);
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
		return err
	}

	sqlQuery = `CREATE TRIGGER onTagDelete BEFORE DELETE ON Tags
	FOR EACH ROW BEGIN
		DELETE FROM ImageTags WHERE TagID=OLD.ID;
		DELETE FROM CollectionTags WHERE TagID=OLD.ID;
	END`
	if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
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
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database columns", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 1;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE Images SET Rating = 'unrated';")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update rating on images after update", err.Error()})
			//Update was technically successfull, not returning for this error
		}
		version = 1
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 1->2
	if version == 1 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD COLUMN (ScoreTotal BIGINT NOT NULL DEFAULT 0, ScoreAverage BIGINT NOT NULL DEFAULT 0, ScoreVoters BIGINT NOT NULL DEFAULT 0);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database columns", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("CREATE TABLE ImageUserScores (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, UserID BIGINT UNSIGNED NOT NULL, ImageID BIGINT UNSIGNED NOT NULL, Score BIGINT NOT NULL, CreationTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageUserPair (UserID,ImageID));")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 2;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 2
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}

	//Update version 2->3
	if version == 2 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD COLUMN (Source VARCHAR(2000) NOT NULL DEFAULT '');")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database columns", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 3;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 3
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 3->4
	if version == 3 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE Users ADD COLUMN (SearchFilter VARCHAR(255) NOT NULL DEFAULT '');")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database columns", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 4;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 4
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 4->5
	if version == 4 {
		//Collections
		_, err := DBConnection.DBHandle.Exec("CREATE TABLE Collections (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, Name VARCHAR(255) NOT NULL UNIQUE, Description VARCHAR(255), UploaderID BIGINT UNSIGNED NOT NULL, UploadTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("CREATE TABLE CollectionMembers (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, ImageID BIGINT UNSIGNED NOT NULL, CollectionID BIGINT UNSIGNED NOT NULL, LinkerID BIGINT UNSIGNED NOT NULL, LinkTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX ImageCollectionPair (CollectionID,ImageID), OrderWeight BIGINT UNSIGNED NOT NULL);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 5;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 5
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 5->6
	if version == 5 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE AuditLogs ADD COLUMN (LogTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("ALTER TABLE AuditLogs CHANGE COLUMN Info Info VARCHAR(10240) NOT NULL DEFAULT '';")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 6;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 6
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 6->7
	if version == 6 {
		_, err := DBConnection.DBHandle.Exec("CREATE TABLE CollectionTags (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, CollectionID BIGINT UNSIGNED NOT NULL, TagID BIGINT UNSIGNED NOT NULL, LinkerID BIGINT UNSIGNED NOT NULL, LinkTime TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL, UNIQUE INDEX CollectionTagPair (TagID,CollectionID));")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		sqlQuery := `CREATE PROCEDURE LinkCollTags(IN collID BIGINT UNSIGNED)
		BEGIN
		-- Insert missing tags
		INSERT INTO CollectionTags (TagID, CollectionID, LinkerID)
		SELECT DISTINCT(ImageTags.TagID), collID, ImageTags.LinkerID
		FROM ImageTags
		INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
		WHERE CollectionMembers.CollectionID = collID
		AND TagID NOT IN (SELECT TagID from CollectionTags WHERE CollectionID = collID);
		-- Remove extra tags
		DELETE FROM CollectionTags
		WHERE TagID NOT IN ( SELECT TagID 
							 FROM ImageTags 
							 INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
							 WHERE CollectionMembers.CollectionID = collID
						   )
		AND CollectionID=collID;
		END`

		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		sqlQuery = `CREATE EVENT auditCleanup 
		ON SCHEDULE EVERY 1 DAY 
		DO 
		DELETE FROM AuditLogs WHERE LogTime < DATE_SUB(current_timestamp(), INTERVAL 30 DAY);`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onCollectionDelete BEFORE DELETE ON Collections
		FOR EACH ROW BEGIN
			DELETE FROM CollectionMembers WHERE CollectionID=OLD.ID;
			DELETE FROM CollectionTags WHERE CollectionID=OLD.ID;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onCollectionMemberAdd AFTER INSERT ON CollectionMembers
		FOR EACH ROW BEGIN
			CALL LinkCollTags(NEW.CollectionID);
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onCollectionMemberDelete AFTER DELETE ON CollectionMembers
		FOR EACH ROW BEGIN
			CALL LinkCollTags(OLD.CollectionID);
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onImageTagDelete AFTER DELETE ON ImageTags
		FOR EACH ROW BEGIN
			DECLARE collID BIGINT UNSIGNED;
			DECLARE cursorDone BOOL DEFAULT FALSE;
			DECLARE collCursor CURSOR FOR SELECT CollectionID FROM CollectionMembers WHERE ImageID = OLD.ImageID;
			DECLARE CONTINUE HANDLER FOR NOT FOUND SET cursorDone = TRUE;
			OPEN collCursor;
			collLoop: LOOP
				FETCH collCursor INTO collID;
				IF cursorDone THEN
					LEAVE collLoop;
				END IF;
				CALL LinkCollTags(collID);
			END LOOP;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onImageDelete BEFORE DELETE ON Images
		FOR EACH ROW BEGIN
			DELETE FROM ImageTags WHERE ImageID=OLD.ID;
			DELETE FROM ImageUserScores WHERE ImageID=OLD.ID;
			DELETE FROM CollectionMembers WHERE ImageID=OLD.ID;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onImageTagInsert AFTER INSERT ON ImageTags
		FOR EACH ROW BEGIN
			DECLARE collID BIGINT UNSIGNED;
			DECLARE cursorDone BOOL DEFAULT FALSE;
			DECLARE collCursor CURSOR FOR SELECT CollectionID FROM CollectionMembers WHERE ImageID = NEW.ImageID;
			DECLARE CONTINUE HANDLER FOR NOT FOUND SET cursorDone = TRUE;
			OPEN collCursor;
			collLoop: LOOP
				FETCH collCursor INTO collID;
				IF cursorDone THEN
					LEAVE collLoop;
				END IF;
				CALL LinkCollTags(collID);
			END LOOP;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onTagDelete BEFORE DELETE ON Tags
		FOR EACH ROW BEGIN
			DELETE FROM ImageTags WHERE TagID=OLD.ID;
			DELETE FROM CollectionTags WHERE TagID=OLD.ID;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		if _, err := DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 7;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 7
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 7->8
	if version == 7 {
		_, err := DBConnection.DBHandle.Exec("DROP PROCEDURE IF EXISTS LinkCollTags;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		sqlQuery := `CREATE PROCEDURE LinkCollTags(IN collID BIGINT UNSIGNED)
		BEGIN
		-- Insert missing tags
		INSERT INTO CollectionTags (TagID, CollectionID, LinkerID)
		SELECT DISTINCT ImageTags.TagID, collID, ImageTags.LinkerID
		FROM ImageTags
		INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
		LEFT JOIN CollectionTags on CollectionTags.CollectionID = CollectionMembers.CollectionID AND CollectionTags.TagID = ImageTags.TagID
		WHERE CollectionMembers.CollectionID = collID AND CollectionTags.CollectionID IS NULL;
		-- Remove extra tags
		DELETE FROM CollectionTags
		WHERE TagID NOT IN ( SELECT TagID 
								FROM ImageTags 
								INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
								WHERE CollectionMembers.CollectionID = collID
							)
		AND CollectionID=collID;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 8;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 8
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 8->9
	if version == 8 {
		if _, err := DBConnection.DBHandle.Exec("ALTER TABLE ImageTags ADD INDEX(ImageID);"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("ALTER TABLE ImageTags ADD INDEX(LinkerID);"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD INDEX(UploaderID);"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD INDEX(Rating);"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD INDEX(UploadTime);"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD INDEX(ScoreAverage);"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery := `CREATE PROCEDURE AddMissingCollectionImageTags(IN imgID BIGINT UNSIGNED)
		BEGIN
			-- Insert missing tags
			INSERT INTO CollectionTags(TagID, CollectionID, LinkerID)
		SELECT DISTINCT
			ImageTags.TagID,
			CollectionMembers.CollectionID,
			ImageTags.LinkerID
		FROM
			ImageTags
		INNER JOIN CollectionMembers ON CollectionMembers.ImageID = ImageTags.ImageID
		LEFT JOIN CollectionTags ON CollectionTags.CollectionID = CollectionMembers.CollectionID AND CollectionTags.TagID = ImageTags.TagID
		WHERE
			CollectionTags.CollectionID IS NULL AND ImageTags.ImageID = imgID;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE PROCEDURE RemSurplusCollectionImageTags(IN collID BIGINT UNSIGNED)
		BEGIN
			-- Remove extra tags
			DELETE FROM CollectionTags
			WHERE TagID NOT IN ( SELECT TagID 
									FROM ImageTags 
									INNER JOIN CollectionMembers on CollectionMembers.ImageID = ImageTags.ImageID
									WHERE CollectionMembers.CollectionID = collID
								)
			AND CollectionID=collID;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("DROP TRIGGER IF EXISTS onImageTagInsert;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		if _, err := DBConnection.DBHandle.Exec("DROP TRIGGER IF EXISTS onCollectionMemberAdd;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		if _, err := DBConnection.DBHandle.Exec("DROP TRIGGER IF EXISTS onImageTagDelete;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		if _, err := DBConnection.DBHandle.Exec("DROP TRIGGER IF EXISTS onCollectionMemberDelete;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onImageTagDelete AFTER DELETE ON ImageTags
		FOR EACH ROW BEGIN
			DECLARE collID BIGINT UNSIGNED;
			DECLARE cursorDone BOOL DEFAULT FALSE;
			DECLARE collCursor CURSOR FOR SELECT CollectionID FROM CollectionMembers WHERE ImageID = OLD.ImageID;
			DECLARE CONTINUE HANDLER FOR NOT FOUND SET cursorDone = TRUE;
			OPEN collCursor;
			collLoop: LOOP
				FETCH collCursor INTO collID;
				IF cursorDone THEN
					LEAVE collLoop;
				END IF;
				CALL RemSurplusCollectionImageTags(collID);
			END LOOP;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onCollectionMemberDelete AFTER DELETE ON CollectionMembers
		FOR EACH ROW BEGIN
			CALL RemSurplusCollectionImageTags(OLD.CollectionID);
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onImageTagInsert AFTER INSERT ON ImageTags
		FOR EACH ROW BEGIN
			CALL AddMissingCollectionImageTags(NEW.ImageID);
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		sqlQuery = `CREATE TRIGGER onCollectionMemberAdd AFTER INSERT ON CollectionMembers
		FOR EACH ROW BEGIN
			CALL AddMissingCollectionImageTags(NEW.ImageID);
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 9;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 9
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 9->10
	if version == 9 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE Images ADD COLUMN (Description VARCHAR(1024) DEFAULT '');")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database columns", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 10;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 10
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 10->11
	if version == 10 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE Images MODIFY Description TEXT NOT NULL DEFAULT '';")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database columns", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 11;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 11
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 11->12
	if version == 11 {
		_, err := DBConnection.DBHandle.Exec("CREATE TABLE ImagedHashes (ID BIGINT UNSIGNED NOT NULL AUTO_INCREMENT UNIQUE, ImageID BIGINT UNSIGNED NOT NULL, vHash BIGINT UNSIGNED NOT NULL, hHash BIGINT UNSIGNED NOT NULL, UNIQUE INDEX(ImageID), INDEX(vHash), INDEX(hHash));")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
			return version, err
		}
		if _, err := DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 12;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 12
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	//Update version 12->13
	if version == 12 {
		_, err := DBConnection.DBHandle.Exec("ALTER TABLE ImagedHashes ADD CONSTRAINT fk_ImagedHashesImageID FOREIGN KEY (ImageID) REFERENCES Images(ID);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("ALTER TABLE CollectionMembers ADD CONSTRAINT fk_CollectionMembersImageID FOREIGN KEY (ImageID) REFERENCES Images(ID), ADD CONSTRAINT fk_CollectionMembersCollectionID FOREIGN KEY (CollectionID) REFERENCES Collections(ID);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("ALTER TABLE CollectionTags ADD CONSTRAINT fk_CollectionTagsCollectionID FOREIGN KEY (CollectionID) REFERENCES Collections(ID), ADD CONSTRAINT fk_CollectionTagsTagID FOREIGN KEY (TagID) REFERENCES Tags(ID);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
			return version, err
		}
		_, err = DBConnection.DBHandle.Exec("ALTER TABLE ImageTags ADD CONSTRAINT fk_ImageTagsImageID FOREIGN KEY (ImageID) REFERENCES Images(ID), ADD CONSTRAINT fk_ImageTagsTagID FOREIGN KEY (TagID) REFERENCES Tags(ID);")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
			return version, err
		}

		_, err = DBConnection.DBHandle.Exec("DROP TRIGGER onImageDelete;")
		if err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/performFreshDBInstall", "", logging.ResultFailure, []string{"Failed to install database", err.Error()})
			return version, err
		}

		sqlQuery := `CREATE TRIGGER onImageDelete BEFORE DELETE ON Images
		FOR EACH ROW BEGIN
			DELETE FROM ImageTags WHERE ImageID=OLD.ID;
			DELETE FROM ImageUserScores WHERE ImageID=OLD.ID;
			DELETE FROM CollectionMembers WHERE ImageID=OLD.ID;
			DELETE FROM ImagedHashes WHERE ImageID=OLD.ID;
		END`
		if _, err := DBConnection.DBHandle.Exec(sqlQuery); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}

		if _, err := DBConnection.DBHandle.Exec("UPDATE DBVersion SET version = 13;"); err != nil {
			logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultFailure, []string{"Failed to update database version", err.Error()})
			return version, err
		}
		version = 13
		logging.WriteLog(logging.LogLevelError,"MariaDBPlugin/InitDatabase", "", logging.ResultInfo, []string{"Database schema updated to version", strconv.FormatInt(version, 10)})
	}
	return version, nil
}

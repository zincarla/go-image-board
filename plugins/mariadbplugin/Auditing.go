package mariadbplugin

import (
	"go-image-board/logging"
	"strconv"
)

//AddAuditLog adds an audit event into the audit table
func (DBConnection *MariaDBPlugin) AddAuditLog(UserID uint64, Type string, Info string) error {
	if len(Type) > 40 || len(Info) > 10240 {

		logging.WriteLog(logging.LogLevelError, "MariaDBPlugin/AddAuditLog", strconv.FormatUint(UserID, 10), logging.ResultFailure, []string{"either the type, or the info is too long for the audit log table", Type, Info})
		Info = Info[:10240]
		Type = Type[:40]
		//return errors.New("either the type, or the info is too long for the audit log table")
	}

	_, err := DBConnection.DBHandle.Exec("INSERT INTO AuditLogs (UserID, Type, Info) VALUES (?, ?, ?);", UserID, Type, Info)
	return err
}

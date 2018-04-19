package mariadbplugin

import (
	"errors"
)

//AddAuditLog adds an audit event into the audit table
func (DBConnection *MariaDBPlugin) AddAuditLog(UserID uint64, Type string, Info string) error {
	if len(Type) > 40 || len(Info) > 255 {
		return errors.New("either the type, or the info is too long for the audit log table")
	}

	_, err := DBConnection.DBHandle.Exec("INSERT INTO AuditLogs (UserID, Type, Info) VALUES (?, ?, ?);", UserID, Type, Info)
	return err
}

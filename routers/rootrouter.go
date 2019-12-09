package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
)

//RootRouter serves requests to the root (/)
func RootRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}
	replyWithTemplate("indextemplate.html", TemplateInput, responseWriter)
}

//BadConfigRouter is served when the config failed to load
func BadConfigRouter(responseWriter http.ResponseWriter, request *http.Request) {
	logging.LogInterface.WriteLog("ContentRouter", "BadConfigRouter", "*", "SUCCESS", []string{config.JoinPath(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"updateconfig.html")})
	http.ServeFile(responseWriter, request, config.JoinPath(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"updateconfig.html"))
}

//WriteAuditLog Writes to the DB audit log, requires userID
func WriteAuditLog(UserID uint64, Type string, Info string) error {

	if err := database.DBInterface.AddAuditLog(UserID, Type, Info); err != nil {
		sid := strconv.FormatUint(UserID, 10)
		logging.LogInterface.WriteLog("RootRouter", "writeAuditLog", sid, "ERROR", []string{"Failed to write audit entry.", err.Error(), Type, Info})
		return err
	}
	return nil
}

//WriteAuditLogByName Writes to the DB audit log, requires UserName
func WriteAuditLogByName(UserName string, Type string, Info string) error {
	userID, err := database.DBInterface.GetUserID(UserName)
	if err != nil {
		logging.LogInterface.WriteLog("RootRouter", "writeAuditLog", UserName, "ERROR", []string{"Could not get user id for audit log.", err.Error(), Type, Info})
	}
	return WriteAuditLog(userID, Type, Info)
}

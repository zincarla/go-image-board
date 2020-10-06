package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"net/http"
	"net/url"
	"path"
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
	logging.WriteLog(logging.LogLevelVerbose, "rootrouter/BadConfigRouter", "", logging.ResultSuccess, []string{path.Join(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"updateconfig.html")})
	//Do not cache this file
	//Otherwise can cause headaches once issue is fixed and server is rebooted as client will just reshow config instead of working service
	responseWriter.Header().Add("Cache-Control", "no-cache, private, max-age=0")
	responseWriter.Header().Add("Pragma", "no-cache")
	responseWriter.Header().Add("X-Accel-Expires", "0")
	http.ServeFile(responseWriter, request, path.Join(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"updateconfig.html"))
}

//WriteAuditLog Writes to the DB audit log, requires userID
func WriteAuditLog(UserID uint64, Type string, Info string) error {

	if err := database.DBInterface.AddAuditLog(UserID, Type, Info); err != nil {
		sid := strconv.FormatUint(UserID, 10)
		logging.WriteLog(logging.LogLevelError, "rootrouter/writeAuditLog", sid, logging.ResultFailure, []string{"Failed to write audit entry.", err.Error(), Type, Info})
		return err
	}
	return nil
}

//WriteAuditLogByName Writes to the DB audit log, requires UserName
func WriteAuditLogByName(UserName string, Type string, Info string) error {
	userID, err := database.DBInterface.GetUserID(UserName)
	if err != nil {
		logging.WriteLog(logging.LogLevelError, "rootrouter/writeAuditLog", UserName, logging.ResultFailure, []string{"Could not get user id for audit log.", err.Error(), Type, Info})
	}
	return WriteAuditLog(userID, Type, Info)
}

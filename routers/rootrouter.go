package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"net/http"
	"path/filepath"
	"strconv"
)

//RootRouter serves requests to the root (/)
func RootRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	replyWithTemplate("indextemplate.html", TemplateInput, responseWriter)
}

//BadConfigRouter is served when the config failed to load
func BadConfigRouter(responseWriter http.ResponseWriter, request *http.Request) {
	logging.LogInterface.WriteLog("ContentRouter", "BadConfigRouter", "*", "SUCCESS", []string{config.JoinPath(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"updateconfig.html")})
	http.ServeFile(responseWriter, request, config.JoinPath(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"updateconfig.html"))
}

//
func writeAuditLog(UserID uint64, Type string, Info string) error {

	if err := database.DBInterface.AddAuditLog(UserID, Type, Info); err != nil {
		sid := strconv.FormatUint(UserID, 10)
		logging.LogInterface.WriteLog("RootRouter", "writeAuditLog", sid, "ERROR", []string{"Failed to write audit entry.", err.Error(), Type, Info})
		return err
	}
	return nil
}

func writeAuditLogByName(UserName string, Type string, Info string) error {
	userID, err := database.DBInterface.GetUserID(UserName)
	if err != nil {
		logging.LogInterface.WriteLog("RootRouter", "writeAuditLog", UserName, "ERROR", []string{"Could not get user id for audit log.", err.Error(), Type, Info})
	}
	return writeAuditLog(userID, Type, Info)
}

package api

import (
	"encoding/json"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"go-image-board/routers"
	"net"
	"net/http"
	"strings"
)

//logonInput is the object that is expected from the client
type logonInput struct {
	Username string
	Password string
}

//LogonAPIRouter serves requests to /api/Logon
func LogonAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//TODO: Brute force weakness here. We need to throttle this somehow....
	//TODO: Come to think of it, should lock out accounts after so many bad password attempts....

	//Decode user request
	decoder := json.NewDecoder(request.Body)
	var logonData logonInput
	if err := decoder.Decode(&logonData); err != nil {
		ReplyWithJSONError(responseWriter, request, "Failed to parse request data, expecting JSON of Username and Password", "*", http.StatusBadRequest)
		return
	}
	//Parse user logon request
	logonData.Username = strings.ToLower(logonData.Username)
	ip, _, _ := net.SplitHostPort(request.RemoteAddr)
	if logonData.Username != "" && logonData.Password != "" {
		err := database.DBInterface.ValidateUser(logonData.Username, []byte(logonData.Password))
		if err == nil {
			//Get Session
			session, _ := config.SessionStore.Get(request, config.SessionVariableName)

			// Set some session values.
			Token, err := database.DBInterface.GenerateToken(logonData.Username, ip)
			if err != nil {
				logging.LogInterface.WriteLog("AccountLogonAPIRouter", "LogonHandler", logonData.Username, "ERROR", []string{"Account Validation", err.Error()})
				ReplyWithJSONError(responseWriter, request, "failed to generate token", logonData.Username, http.StatusInternalServerError)
				return
			}
			session.Values["TokenID"] = Token
			session.Values["UserName"] = logonData.Username
			// Save it before we write to the response/return from the handler.
			session.Save(request, responseWriter)
			go routers.WriteAuditLogByName(logonData.Username, "LOGON", logonData.Username+" successfully logged on with API.")
			logging.LogInterface.WriteLog("AccountLogonAPIRouter", "LogonHandler", logonData.Username, "SUCCESS", []string{"Account Validation"})
			ReplyWithJSON(responseWriter, request, GenericResponse{Result: "Successfully signed in"}, logonData.Username)
			return
		}
		go routers.WriteAuditLogByName(logonData.Username, "LOGON", logonData.Username+" failed to log in. "+err.Error())
		responseWriter.Header().Add("WWW-Authenticate", "Newauth realm=\"gib-api\"")
		ReplyWithJSONError(responseWriter, request, "wrong username or password", "*", http.StatusUnauthorized)
		return
	}
	ReplyWithJSONError(responseWriter, request, "either username or password was blank", "*", http.StatusBadRequest)
	return
}

//LogoutAPIRouter serves requests to /api/Logout
func LogoutAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	_, UserName, TokenID := routers.ValidateUserLogon(request)

	//Grab and clear session variables
	session, _ := config.SessionStore.Get(request, config.SessionVariableName)
	session.Values["TokenID"] = ""
	session.Values["UserName"] = ""
	session.Save(request, responseWriter)

	//But only wipe token from DB if session info was correct
	//The idea is to prevent a DOS in the event someone constructs an invalid session
	if TokenID != "" {
		err := database.DBInterface.RevokeToken(UserName)
		if err != nil {
			logging.LogInterface.WriteLog("AccountLogoutAPIRouter", "LogoutHandler", UserName, "WARN", []string{"Account logout was requested but an error occured during token removal", err.Error()})
		}
	}
	go routers.WriteAuditLogByName(UserName, "LOGOUT", UserName+" manually logged out.")
	ReplyWithJSON(responseWriter, request, GenericResponse{Result: "you have been logged out"}, UserName)
}

package api

import (
	"encoding/binary"
	"encoding/json"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"go-image-board/routers"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//logonInput is the object that is expected from the client
type logonInput struct {
	Username string
	Password string
}

//IPThrottle is a thread-safe cache of API throttle times, specifically for the logon events
var IPThrottle ThrottleMap

//ThrottleAPILogonIP This helper function shortens code elsewhere. This validates whether an ip is in a throttle period, and if so, tells them.
func ThrottleAPILogonIP(responseWriter http.ResponseWriter, request *http.Request) bool {
	//Convert IP to UInt64 to be compliant with ThrottleMap
	ipString, _, _ := net.SplitHostPort(request.RemoteAddr)
	ip := net.ParseIP(ipString)
	var UserIP uint64
	if ip != nil {
		UserIP = binary.BigEndian.Uint64(ip)
	}
	//Respond with TooManyRequests if user is throttled, otherwise let parent function continue
	canUse, throttleTime := IPThrottle.CanUseAPI(UserIP)
	if canUse {
		//This may not be best place for this, but seems simplest. We will set the throttle here, since this is called at the top of all API requests
		//This also ensure throttle is only set on a non-throttled attempt and not reset inbetween
		Throttle.SetValue(UserIP, 5000) //Hard 5 second throttle
		return true                     //User logged in, and not throttled, tell calling function to continue
	}
	retrySeconds := int64(throttleTime.Sub(time.Now()).Seconds())
	if retrySeconds <= 0 {
		retrySeconds = 1
	}
	responseWriter.Header().Set("Retry-After", strconv.FormatInt(retrySeconds, 10))
	ReplyWithJSONStatus(responseWriter, request, ThrottleErrorResponse{Error: "Please wait a bit between requests", Timeout: throttleTime.Sub(time.Now()).Milliseconds()}, ipString, http.StatusTooManyRequests)
	return false
}

//LogonAPIRouter serves requests to /api/Logon
func LogonAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//TODO: Come to think of it, should lock out accounts after so many bad password attempts....

	//Logon throttle check
	if ThrottleAPILogonIP(responseWriter, request) == false {
		return
	}

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

package api

import (
	"encoding/json"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//ErrorResponse used to marshal error text for JSON parsing
type ErrorResponse struct {
	Error string
}

//ThrottleErrorResponse used to reply error text and time in milliseconds
type ThrottleErrorResponse struct {
	Error   string
	Timeout int64
}

//GenericResponse used to reply with a simple text-based result
type GenericResponse struct {
	Result string
}

//ReplyWithJSON replies to a request with the specified interface to be marshaled to a JOSN object
func ReplyWithJSON(responseWriter http.ResponseWriter, request *http.Request, jsonObject interface{}, userName string) {
	ReplyWithJSONStatus(responseWriter, request, jsonObject, userName, http.StatusOK)
}

//ReplyWithJSONStatus replies to a request with the specified interface to be marshaled to a JOSN object and a custom status code
func ReplyWithJSONStatus(responseWriter http.ResponseWriter, request *http.Request, jsonObject interface{}, userName string, statusCode int) {
	response, err := json.Marshal(jsonObject)
	if err != nil {
		logging.LogInterface.WriteLog("APIRouter", "ReplyWithJSONStatus", userName, "ERROR", []string{"Error generating JSON during reply", err.Error()})
		http.Error(responseWriter, "internal error generating response", http.StatusInternalServerError)
		return
	}
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)
	responseWriter.Write(response)
}

//ReplyWithJSONError replies to a request with an error response
func ReplyWithJSONError(responseWriter http.ResponseWriter, request *http.Request, errorText string, userName string, statusCode int) {
	logging.LogInterface.WriteLog("APIRoot", "ReplyWithJSONError", userName, "ERROR", []string{errorText})
	ReplyWithJSONStatus(responseWriter, request, ErrorResponse{Error: errorText}, userName, statusCode)
}

//ValidateAPIUser This helper function shortens code elsewhere. This is generally called with every API request. Returns ShouldContinue, UserID, UserName.
func ValidateAPIUser(responseWriter http.ResponseWriter, request *http.Request) (bool, uint64, string) {
	//Validate Logon
	UserID, UserName, TokenID := routers.ValidateUserLogon(request)
	errMSG := ""
	if UserID != 0 && UserName != "" && TokenID != "" {
		//Validated with session
		return true, UserID, UserName
	}
	//Attempt with auth header instead
	authHeader := request.Header.Get("Authorization")
	if authHeader != "" {
		//Remove "Newauth" from header
		if strings.HasPrefix(authHeader, "Newauth ") {
			authHeader = authHeader[8:]
			authPieces := strings.Split(authHeader, ":")
			if len(authPieces) == 2 {
				userName := authPieces[0]
				tokenID := authPieces[1]
				ip, _, _ := net.SplitHostPort(request.RemoteAddr)
				userID, err := database.DBInterface.GetUserID(userName)
				if err == nil {
					if err := database.DBInterface.ValidateToken(userName, tokenID, ip); err != nil {
						logging.LogInterface.WriteLog("APIRoot", "ValidateAPIUser", userName, "SUCCESS", []string{"Validated by header"})
						return true, userID, userName //Valid auth header
					}
					errMSG = "Token is invalid"
				}
				errMSG = "User is invalid"
			}
		} else {
			errMSG = "Auth header incorrect format"
		}
	}

	responseWriter.Header().Add("WWW-Authenticate", "Newauth realm=\"gib-api\"") //IANA requires a WWW-Authenticate header with StatusUnauthorized
	ReplyWithJSONError(responseWriter, request, "Unauthenticated request, please login first. "+errMSG, "*", http.StatusUnauthorized)
	return false, 0, ""
}

//ThrottleAPIUser This helper function shortens code elsewhere. This validates whether a user is in a throttle period, and if so, tells them.
func ThrottleAPIUser(responseWriter http.ResponseWriter, request *http.Request, UserName string, UserID uint64) bool {
	//Validate API Throttle
	//Responde with TooManyRequests if user is throttled, otherwise let parent function continue
	canUse, throttleTime := Throttle.CanUseAPI(UserID)
	if canUse {
		//This may not be best place for this, but seems simplest. We will set the throttle here, since this is called at the top of all API requests
		//This also ensure throttle is only set on a non-throttled attempt and not reset inbetween
		Throttle.SetValue(UserID, config.Configuration.APIThrottle)
		return true //User logged in, and not throttled, tell calling function to continue
	}
	retrySeconds := int64(throttleTime.Sub(time.Now()).Seconds())
	if retrySeconds <= 0 {
		retrySeconds = 1
	}
	responseWriter.Header().Set("Retry-After", strconv.FormatInt(retrySeconds, 10))
	ReplyWithJSONStatus(responseWriter, request, ThrottleErrorResponse{Error: "Please wait a bit between requests", Timeout: throttleTime.Sub(time.Now()).Milliseconds()}, UserName, http.StatusTooManyRequests)
	return false
}

//ValidateAndThrottleAPIUser Shorthand function to call both ValidateAPIUser and ThrottleAPIUser
func ValidateAndThrottleAPIUser(responseWriter http.ResponseWriter, request *http.Request) (bool, uint64, string) {
	goodLogin, userID, userName := ValidateAPIUser(responseWriter, request)
	if goodLogin {
		goodThrottle := ThrottleAPIUser(responseWriter, request, userName, userID)
		return goodThrottle, userID, userName
	}
	return false, userID, userName
}

//ValidateAPIUserWriteAccess Validates the given user has permission to use advanced API functions, and if not repsonds to user. Returns ShouldContinue, and UserPermissions
func ValidateAPIUserWriteAccess(responseWriter http.ResponseWriter, request *http.Request, UserName string) (bool, interfaces.UserPermission) {
	//Get user permission info
	permissions, err := database.DBInterface.GetUserPermissionSet(UserName)
	if err != nil {
		ReplyWithJSONError(responseWriter, request, "Could not validate your permission, internal databse error", UserName, http.StatusForbidden)
		return false, permissions
	}
	//Validate Permission to delete using api
	if interfaces.UserPermission(permissions).HasPermission(interfaces.APIWriteAccess) != true {
		ReplyWithJSONError(responseWriter, request, "You do not have API write access", UserName, http.StatusForbidden)
		go routers.WriteAuditLogByName(UserName, "API", UserName+" failed query API. Insufficient permissions. ")
		return false, permissions
	}
	return true, permissions
}

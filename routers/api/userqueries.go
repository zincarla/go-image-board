package api

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"strconv"
	"strings"
)

//UserSearchResult contains information for a user
type UserSearchResult struct {
	Users        []interfaces.UserInformation
	ResultCount  uint64
	ServerStride uint64
}

//UsersAPIRouter serves requests to /api/Users
func UsersAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//Query for a user's information, will return UserInformation
	requestedName := strings.TrimSpace(request.FormValue("userNameQuery"))
	//Get the page offset
	pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
	pageStride := config.Configuration.PageStride

	//Validate Permission
	////Get User Info
	UserPerms, err := database.DBInterface.GetUserPermissionSet(UserName)
	if UserPerms.HasPermission(interfaces.EditUserPermissions) == false && UserPerms.HasPermission(interfaces.DisableUser) == false {
		ReplyWithJSONError(responseWriter, request, "Authenticated, but insufficient permissions to perform request", UserName, http.StatusForbidden)
		return
	}

	//Perform Query
	userInfo, count, err := database.DBInterface.SearchUsers(requestedName, pageStart, pageStride)
	if err != nil {
		logging.WriteLog(logging.LogLevelError, "userqueries/UserAPIRouter", UserName, logging.ResultFailure, []string{"Failed to query users", err.Error()})
		ReplyWithJSONError(responseWriter, request, "Internal Database Error Occured", UserName, http.StatusInternalServerError)
		return
	}

	ReplyWithJSON(responseWriter, request, UserSearchResult{Users: userInfo, ResultCount: count, ServerStride: pageStride}, UserName)
}

package api

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers"
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

//UserAPIRouter serves requests to /api/User
func UserAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserID, UserName, TokenID := routers.ValidateUserLogon(request)
	if UserID == 0 || UserName == "" || TokenID == "" {
		http.Error(responseWriter, "Unauthenticated request, please login first", http.StatusUnauthorized)
		return
	}

	if request.Method == "GET" {
		//Query for a collection's information, will return CollectionInformation
		requestedName := strings.TrimSpace(request.FormValue("userNameQuery"))
		//Get the page offset
		pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
		pageStride := config.Configuration.PageStride

		//Validate Permission
		////Get User Info
		UserPerms, err := database.DBInterface.GetUserPermissionSet(UserName)
		if UserPerms.HasPermission(interfaces.EditUserPermissions) == false && UserPerms.HasPermission(interfaces.DisableUser) == false {
			http.Error(responseWriter, "Authenticated, but insufficient permissions to perform request", http.StatusUnauthorized)
			return
		}

		//Perform Query
		userInfo, count, err := database.DBInterface.SearchUsers(requestedName, pageStart, pageStride)
		if err != nil {
			logging.LogInterface.WriteLog("UserQueryAPI", "UserAPIRouter", UserName, "ERROR", []string{"Failed to query users", err.Error()})
			http.Error(responseWriter, "SQL Error", http.StatusInternalServerError)
			return
		}

		ReplyWithJSON(responseWriter, request, UserSearchResult{Users: userInfo, ResultCount: count, ServerStride: pageStride}, UserName)
	}
}

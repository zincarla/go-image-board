package api

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"strconv"
)

//CollectionSearchResult response format for a collection search
type CollectionSearchResult struct {
	Collections  []interfaces.CollectionInformation
	ResultCount  uint64
	ServerStride uint64
}

//CollectionsGetAPIRouter serves get requests to /api/Collections
func CollectionsGetAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, UserID, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//Query for a collection's information, will return CollectionInformation
	userQuery := request.FormValue("SearchQuery")
	pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
	pageStride := config.Configuration.PageStride

	userQTags, err := database.DBInterface.GetQueryTags(userQuery, true)
	if err == nil {
		//add user's global filters to query
		userFilterTags, err := database.DBInterface.GetUserFilterTags(UserID, true)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "collectionqueries/CollectionsAPIRouter", UserName, logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
		} else {
			userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
		}

		//Perform Query
		collectionInfo, MaxCount, err := database.DBInterface.SearchCollections(userQTags, pageStart, pageStride)
		ReplyWithJSON(responseWriter, request, CollectionSearchResult{Collections: collectionInfo, ResultCount: MaxCount, ServerStride: pageStride}, UserName)
		return
	}
	logging.WriteLog(logging.LogLevelError, "collectionqueries/CollectionsAPIRouter", UserName, logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
	ReplyWithJSONError(responseWriter, request, "failed to parse your query", UserName, http.StatusInternalServerError)
}

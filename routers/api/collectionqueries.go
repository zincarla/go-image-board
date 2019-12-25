package api

import (
	"database/sql"
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

//CollectionNameAPIRouter serves requests to /api/CollectionName
func CollectionNameAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	if request.Method == http.MethodGet {
		//Query for a collection's information, will return CollectionInformation
		requestedName := request.FormValue("CollectionName")

		if requestedName != "" {
			collection, err := database.DBInterface.GetCollectionByName(requestedName)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No collection found by that Name", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Internal Database Error", UserName, http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, collection, UserName)
		} else {
			ReplyWithJSONError(responseWriter, request, "Please specify CollectionName", UserName, http.StatusBadRequest)
		}
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

//CollectionAPIRouter serves requests to /api/Collection
func CollectionAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	if request.Method == http.MethodGet {
		//Query for a collection's information, will return CollectionInformation
		requestedID := request.FormValue("CollectionID")
		if requestedID != "" {
			//Grab specific collection by ID
			parsedID, err := strconv.ParseUint(requestedID, 10, 32)
			if err != nil {
				ReplyWithJSONError(responseWriter, request, "CollectionID could not be parsed into a number", UserName, http.StatusBadRequest)
				return
			}
			collection, err := database.DBInterface.GetCollection(parsedID)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No collection by that ID", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, collection, UserName)
		} else {
			ReplyWithJSONError(responseWriter, request, "Please specify CollectionID", UserName, http.StatusBadRequest)
			return
		}
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

//CollectionsAPIRouter serves requests to /api/Collections
func CollectionsAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, UserID, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	if request.Method == http.MethodGet {
		//Query for a collection's information, will return CollectionInformation
		userQuery := request.FormValue("SearchQuery")
		pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
		pageStride := config.Configuration.PageStride

		userQTags, err := database.DBInterface.GetQueryTags(userQuery, true)
		if err == nil {
			//add user's global filters to query
			userFilterTags, err := database.DBInterface.GetUserFilterTags(UserID, true)
			if err != nil {
				logging.LogInterface.WriteLog("API", "CollectionsAPIRouter", UserName, "ERROR", []string{"Failed to load user's filter", err.Error()})
			} else {
				userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
			}

			//Perform Query
			collectionInfo, MaxCount, err := database.DBInterface.SearchCollections(userQTags, pageStart, pageStride)
			ReplyWithJSON(responseWriter, request, CollectionSearchResult{Collections: collectionInfo, ResultCount: MaxCount, ServerStride: pageStride}, UserName)
		} else {
			logging.LogInterface.WriteLog("API", "CollectionsAPIRouter", UserName, "ERROR", []string{"Failed to load user's filter", err.Error()})
			ReplyWithJSONError(responseWriter, request, "failed to parse your query", UserName, http.StatusInternalServerError)
		}
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

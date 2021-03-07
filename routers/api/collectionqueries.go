package api

import (
	"database/sql"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
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
}

//CollectionAPIRouter serves requests to /api/Collection/{CollectionID}
func CollectionAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, UserID, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}
	//Validate Permission to use api
	UserAPIWriteValidated, permissions := ValidateAPIUserWriteAccess(responseWriter, request, UserName)
	if !UserAPIWriteValidated {
		return //User does not have API access and was already told
	}

	//Get variables for URL mux from Gorilla
	urlVariables := mux.Vars(request)

	if request.Method == http.MethodGet {
		//Query for a collection's information, will return CollectionInformation
		requestedID := urlVariables["CollectionID"]
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
	} else if request.Method == http.MethodDelete {
		//Query for a collection's information, will return CollectionInformation
		requestedID := urlVariables["CollectionID"]
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
			//Verify delete permissions
			if interfaces.UserPermission(permissions).HasPermission(interfaces.RemoveCollections) != true && (config.Configuration.UsersControlOwnObjects != true || collection.UploaderID != UserID) {
				ReplyWithJSONError(responseWriter, request, "You do not have permission to delete that", UserName, http.StatusForbidden)
				go routers.WriteAuditLogByName(UserName, "DELETE-IMAGE", UserName+" failed to delete collection with API. Insufficient permissions. "+requestedID)
				return
			}
			//Check if we are to delete members as well
			deleteMembers := request.FormValue("DeletMembers")
			additionalMessages := ""
			if strings.ToLower(deleteMembers) == "true" {
				//Grab list of images
				CollectionMembers, _, err := database.DBInterface.GetCollectionMembers(parsedID, 0, 0)
				if err != nil {
					ReplyWithJSONError(responseWriter, request, "Failed to delete collection. SQL Error getting collection memebers.", UserName, http.StatusInternalServerError)
					go routers.WriteAuditLogByName(UserName, "DELETE-COLLECTION", UserName+" failed to delete collection. "+requestedID+", "+err.Error())
					return
				}

				//Check permissions for all members
				for _, ImageInfo := range CollectionMembers {
					//Validate Permission to delete
					if permissions.HasPermission(interfaces.RemoveImage) != true && (config.Configuration.UsersControlOwnObjects != true || ImageInfo.UploaderID != UserID) {
						ReplyWithJSONError(responseWriter, request, "You do not have permission to delete all members. "+strconv.FormatUint(ImageInfo.ID, 10), UserName, http.StatusForbidden)
						go routers.WriteAuditLogByName(UserName, "DELETE-COLLECTION", UserName+" failed to delete image. Insufficient permissions. "+requestedID)
						return
					}
				}
				//Permission validated for all members, delete them
				for _, ImageInfo := range CollectionMembers {
					err = database.DBInterface.DeleteImage(ImageInfo.ID)
					if err != nil {
						additionalMessages += "Failed to delete collection member " + strconv.FormatUint(ImageInfo.ID, 10) + ". "
						go routers.WriteAuditLogByName(UserName, "DELETE-COLLECTION", UserName+" failed to delete image "+strconv.FormatUint(ImageInfo.ID, 10))
					}
				}
			}
			//Permission validated, delete collection
			if err := database.DBInterface.DeleteCollection(parsedID); err != nil {
				ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
				go routers.WriteAuditLogByName(UserName, "DELETE-COLLECTION", UserName+" failed to delete collection with API. "+requestedID+", "+err.Error())
				return //Cancel delete
			}
			ReplyWithJSON(responseWriter, request, GenericResponse{Result: "Successfully deleted collection " + requestedID + ". " + additionalMessages}, UserName)
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
	//Validate Permission to use api
	UserAPIWriteValidated, _ := ValidateAPIUserWriteAccess(responseWriter, request, UserName)
	if !UserAPIWriteValidated {
		return //User does not have API access and was already told
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
				logging.WriteLog(logging.LogLevelError, "collectionqueries/CollectionsAPIRouter", UserName, logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
			} else {
				userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
			}

			//Perform Query
			collectionInfo, MaxCount, err := database.DBInterface.SearchCollections(userQTags, pageStart, pageStride)
			ReplyWithJSON(responseWriter, request, CollectionSearchResult{Collections: collectionInfo, ResultCount: MaxCount, ServerStride: pageStride}, UserName)
		} else {
			logging.WriteLog(logging.LogLevelError, "collectionqueries/CollectionsAPIRouter", UserName, logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
			ReplyWithJSONError(responseWriter, request, "failed to parse your query", UserName, http.StatusInternalServerError)
		}
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

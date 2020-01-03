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

//TagSearchResult response format for a tag search
type TagSearchResult struct {
	Tags         []interfaces.TagInformation
	ResultCount  uint64
	ServerStride uint64
}

//TagNameAPIRouter serves requests to /api/TagName
func TagNameAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//This is used for auto-complete functionality
	if request.Method == "GET" {
		//Query for a tag's informaion, will return TagInformation
		requestedName := strings.TrimSpace(request.FormValue("tagNameQuery"))

		//Perform Query
		tagInfo, count, err := database.DBInterface.SearchTags(requestedName, 0, 5, true, true)
		if err != nil {
			logging.LogInterface.WriteLog("TagQueries", "TagNameAPIRouter", "*", "ERROR", []string{"Failed to query tags", err.Error()})
			ReplyWithJSONError(responseWriter, request, "Internal Database Error Occured", UserName, http.StatusInternalServerError)
			return
		}

		ReplyWithJSON(responseWriter, request, TagSearchResult{Tags: tagInfo, ResultCount: count, ServerStride: 5}, "*")
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

//TagAPIRouter serves requests to /api/Tag/{TagID}
func TagAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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

	//This is used for auto-complete functionality
	if request.Method == "GET" {
		//Query for a tag's informaion, will return TagInformation
		requestedID := urlVariables["TagID"]

		if requestedID != "" {
			//Grab specific tag by ID
			parsedID, err := strconv.ParseUint(requestedID, 10, 32)
			if err != nil {
				ReplyWithJSONError(responseWriter, request, "TagID could not be parsed into a number", UserName, http.StatusBadRequest)
				return
			}
			tag, err := database.DBInterface.GetTag(parsedID, true)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No tag by that ID", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Internal database error", UserName, http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, tag, UserName)
		} else if request.Method == http.MethodDelete {
			//Grab specific tag by ID
			parsedID, err := strconv.ParseUint(requestedID, 10, 32)
			if err != nil {
				ReplyWithJSONError(responseWriter, request, "TagID could not be parsed into a number", UserName, http.StatusBadRequest)
				return
			}
			tag, err := database.DBInterface.GetTag(parsedID, true)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No tag by that ID", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Internal database error", UserName, http.StatusInternalServerError)
				return
			}

			//Validate delete permissions
			if interfaces.UserPermission(permissions).HasPermission(interfaces.RemoveTags) != true && (config.Configuration.UsersControlOwnObjects != true || tag.UploaderID != UserID) {
				ReplyWithJSONError(responseWriter, request, "You do not have permission to delete that", UserName, http.StatusForbidden)
				go routers.WriteAuditLogByName(UserName, "DELETE-TAG", UserName+" failed to delete tag with API. Insufficient permissions. "+requestedID)
				return
			}

			//Permission validated, now delete
			if err := database.DBInterface.DeleteTag(parsedID); err != nil {
				ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
				go routers.WriteAuditLogByName(UserName, "DELETE-TAG", UserName+" failed to delete tag with API. "+requestedID+", "+err.Error())
				return //Cancel delete
			}
			//Reply Success
			ReplyWithJSON(responseWriter, request, GenericResponse{Result: "Successfully deleted tag " + requestedID}, UserName)
		} else {
			ReplyWithJSONError(responseWriter, request, "Please specify TagID", UserName, http.StatusBadRequest)
			return
		}
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

//TagsAPIRouter serves requests to /api/Tags
func TagsAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User either not logged in, or hit by throttle. Either way, already handled.
	}
	//Validate Permission to use api
	UserAPIWriteValidated, _ := ValidateAPIUserWriteAccess(responseWriter, request, UserName)
	if !UserAPIWriteValidated {
		return //User does not have API access and was already told
	}

	//This is used for auto-complete functionality
	if request.Method == http.MethodGet {
		//Query for a tag's informaion, will return TagInformation
		requestedName := strings.TrimSpace(request.FormValue("tagNameQuery"))
		pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
		pageStride := config.Configuration.PageStride

		//Perform Query
		tagInfo, count, err := database.DBInterface.SearchTags(requestedName, pageStart, pageStride, false, false)
		if err != nil {
			logging.LogInterface.WriteLog("TagQueries", "TagsAPIRouter", "*", "ERROR", []string{"Failed to query tags", err.Error()})
			ReplyWithJSONError(responseWriter, request, "Internal Database Error Occured", UserName, http.StatusInternalServerError)
			return
		}

		ReplyWithJSON(responseWriter, request, TagSearchResult{Tags: tagInfo, ResultCount: count, ServerStride: pageStride}, "*")
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

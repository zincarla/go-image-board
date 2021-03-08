package api

import (
	"database/sql"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/routers"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

//TagGetAPIRouter serves get requests to /api/Tag/{TagID}
func TagGetAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//Get variables for URL mux from Gorilla
	urlVariables := mux.Vars(request)

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
		return
	}
	ReplyWithJSONError(responseWriter, request, "Please specify TagID", UserName, http.StatusBadRequest)
}

//TagDeleteAPIRouter serves delete requests to /api/Tag/{TagID}
func TagDeleteAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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

	//Query for a tag's informaion, will return TagInformation
	requestedID := urlVariables["TagID"]

	if requestedID != "" {
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
		return
	}
	ReplyWithJSONError(responseWriter, request, "Please specify TagID", UserName, http.StatusBadRequest)
}

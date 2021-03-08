package api

import (
	"database/sql"
	"go-image-board/database"
	"net/http"
)

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

package api

import (
	"database/sql"
	"go-image-board/database"
	"go-image-board/routers"
	"net/http"
	"strconv"
)

//CollectionAPIRouter serves requests to /api/Collection
func CollectionAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserID, UserName, TokenID := routers.ValidateUserLogon(request)
	if UserID == 0 || UserName == "" || TokenID == "" {
		http.Error(responseWriter, "Unauthenticated request, please login first", http.StatusUnauthorized)
		return
	}

	if request.Method == "GET" {
		//Query for a collection's information, will return CollectionInformation
		requestedID := request.FormValue("CollectionID")
		requestedName := request.FormValue("CollectionName")
		if requestedID != "" {
			//Grab specific collection by ID
			parsedID, err := strconv.ParseUint(requestedID, 10, 32)
			if err != nil {
				http.Error(responseWriter, "CollectionID could not be parsed into a number", http.StatusBadRequest)
				return
			}
			collection, err := database.DBInterface.GetCollection(parsedID)
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(responseWriter, "No collection by that ID", http.StatusNotFound)
					return
				}
				http.Error(responseWriter, "Error getting collection", http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, collection, UserName)
		} else if requestedName != "" {
			collection, err := database.DBInterface.GetCollectionByName(requestedName)
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(responseWriter, "No collection by that Name", http.StatusNotFound)
					return
				}
				http.Error(responseWriter, "Error getting collection", http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, collection, UserName)
		} else {
			http.Error(responseWriter, "Please specify either CollectionID or CollectionName", http.StatusBadRequest)
			return
		}
	}
}

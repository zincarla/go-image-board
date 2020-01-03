package api

import (
	"database/sql"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gorilla/mux"
)

//ImageSearchResult response format for an image search
type ImageSearchResult struct {
	Images       []interfaces.ImageInformation
	ResultCount  uint64
	ServerStride uint64
}

//ImageAPIRouter serves requests to /api/Image/{ImageID}
func ImageAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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
		//Query for a images's information, will return ImageInformation
		requestedID := urlVariables["ImageID"]
		if requestedID != "" {
			//Grab specific image by ID
			parsedID, err := strconv.ParseUint(requestedID, 10, 32)
			if err != nil {
				ReplyWithJSONError(responseWriter, request, "ImageID could not be parsed into a number", UserName, http.StatusBadRequest)
				return
			}
			image, err := database.DBInterface.GetImage(parsedID)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No image by that ID", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, image, UserName)
		} else {
			ReplyWithJSONError(responseWriter, request, "Please specify ImageID", UserName, http.StatusBadRequest)
			return
		}
	} else if request.Method == http.MethodDelete {
		requestedID := urlVariables["ImageID"]
		if requestedID != "" {
			//Grab specific image by ID
			parsedID, err := strconv.ParseUint(requestedID, 10, 32)
			if err != nil {
				ReplyWithJSONError(responseWriter, request, "ImageID could not be parsed into a number", UserName, http.StatusBadRequest)
				return
			}
			//Get Image info
			imageInfo, err := database.DBInterface.GetImage(parsedID)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No image by that ID", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
				return
			}

			//Validate delete permissions
			if interfaces.UserPermission(permissions).HasPermission(interfaces.RemoveImage) != true && (config.Configuration.UsersControlOwnObjects != true || imageInfo.UploaderID != UserID) {
				ReplyWithJSONError(responseWriter, request, "You do not have permission to delete that", UserName, http.StatusForbidden)
				go routers.WriteAuditLogByName(UserName, "DELETE-IMAGE", UserName+" failed to delete image with API. Insufficient permissions. "+requestedID)
				return
			}

			//Delete
			//Permission validated, now delete (ImageTags and Images)
			if err := database.DBInterface.DeleteImage(parsedID); err != nil {
				ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
				go routers.WriteAuditLogByName(UserName, "DELETE-IMAGE", UserName+" failed to delete image with API. "+requestedID+", "+err.Error())
				return //Cancel delete
			}
			go routers.WriteAuditLogByName(UserName, "DELETE-IMAGE", UserName+" deleted image with API. "+requestedID+", "+imageInfo.Name+", "+imageInfo.Location)
			//Third, delete Image from Disk
			go os.Remove(config.JoinPath(config.Configuration.ImageDirectory, imageInfo.Location))
			//Last delete thumbnail from disk
			go os.Remove(config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+imageInfo.Location+".png"))
			//Reply Success
			ReplyWithJSON(responseWriter, request, GenericResponse{Result: "Successfully deleted image " + requestedID}, UserName)
		} else {
			ReplyWithJSONError(responseWriter, request, "Please specify ImageID", UserName, http.StatusBadRequest)
			return
		}
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

//ImagesAPIRouter serves requests to /api/Images
func ImagesAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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
		//Query for a images's information, will return ImageInformation
		userQuery := request.FormValue("SearchQuery")
		pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
		pageStride := config.Configuration.PageStride

		userQTags, err := database.DBInterface.GetQueryTags(userQuery, true)
		if err == nil {
			//add user's global filters to query
			userFilterTags, err := database.DBInterface.GetUserFilterTags(UserID, true)
			if err != nil {
				logging.LogInterface.WriteLog("API", "ImagesAPIRouter", UserName, "ERROR", []string{"Failed to load user's filter", err.Error()})
			} else {
				userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
			}

			//Perform Query
			imageInfo, MaxCount, err := database.DBInterface.SearchImages(userQTags, pageStart, pageStride)
			ReplyWithJSON(responseWriter, request, ImageSearchResult{Images: imageInfo, ResultCount: MaxCount, ServerStride: pageStride}, UserName)
		} else {
			logging.LogInterface.WriteLog("API", "ImagesAPIRouter", UserName, "ERROR", []string{"Failed to parse user query", err.Error()})
			ReplyWithJSONError(responseWriter, request, "failed to parse your query", UserName, http.StatusInternalServerError)
		}
	} else {
		ReplyWithJSONError(responseWriter, request, "unknown method used", UserName, http.StatusBadRequest)
	}
}

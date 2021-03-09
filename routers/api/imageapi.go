package api

import (
	"database/sql"
	"encoding/json"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/routers"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/gorilla/mux"
)

//ImageGetAPIRouter serves get requests to /api/Image/{ImageID}
func ImageGetAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//Get variables for URL mux from Gorilla
	urlVariables := mux.Vars(request)
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
		return
	}
	ReplyWithJSONError(responseWriter, request, "Please specify ImageID", UserName, http.StatusBadRequest)

}

//ImageDeleteAPIRouter serves delete requests to /api/Image/{ImageID}
func ImageDeleteAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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
		go os.Remove(path.Join(config.Configuration.ImageDirectory, imageInfo.Location))
		//Last delete thumbnail from disk
		go os.Remove(path.Join(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+imageInfo.Location+".png"))
		//Reply Success
		ReplyWithJSON(responseWriter, request, GenericResponse{Result: "Successfully deleted image " + requestedID}, UserName)
		return
	}
	ReplyWithJSONError(responseWriter, request, "Please specify ImageID", UserName, http.StatusBadRequest)
}

type uploadFileInput struct {
	Tags       string
	Source     string
	Collection string
	Files      []routers.UploadingFile
}
type uploadFileReply struct {
	LastID       uint64
	DuplicateIDs map[string]uint64
	Errors       string
}

//ImagePostAPIRouter serves post requests to /api/Image
func ImagePostAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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

	//Verify user can upload an image
	if interfaces.UserPermission(permissions).HasPermission(interfaces.UploadImage) != true {
		go routers.WriteAuditLog(UserID, "IMAGE-UPLOAD", UserName+" failed to upload image. No permissions.")
		ReplyWithJSONError(responseWriter, request, "Insufficient permissions to upload", UserName, http.StatusForbidden)
		return
	}

	//Parse user upload JSON request
	decoder := json.NewDecoder(request.Body)
	var uploadData uploadFileInput
	if err := decoder.Decode(&uploadData); err != nil {
		ReplyWithJSONError(responseWriter, request, "Failed to parse request data", "", http.StatusBadRequest)
		return
	}

	//Send request to HandleImageUploadRequest
	lastID, duplicateIDs, errors := routers.HandleImageUploadRequest(request, interfaces.UserInformation{Name: UserName, ID: UserID}, uploadData.Collection, uploadData.Tags, uploadData.Files, uploadData.Source)

	uploadReply := uploadFileReply{LastID: lastID, DuplicateIDs: duplicateIDs, Errors: errors.Error()}

	ReplyWithJSON(responseWriter, request, uploadReply, UserName)
}

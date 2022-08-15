package api

import (
	"database/sql"
	"encoding/json"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/routers"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

//ImageTagGetResult response format for an tag enumeration on an image
type ImageTagGetResult struct {
	Tags        []interfaces.TagInformation
	ResultCount int
}

//ImageTagsGetAPIRouter serves get requests to /api/Image/{ImageID}/Tags
func ImageTagsGetAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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
		_, err = database.DBInterface.GetImage(parsedID)
		if err != nil {
			if err == sql.ErrNoRows {
				ReplyWithJSONError(responseWriter, request, "No image by that ID", UserName, http.StatusNotFound)
				return
			}
			ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
			return
		}
		tags, err := database.DBInterface.GetImageTags(parsedID)
		if err != nil {
			if err == sql.ErrNoRows {
				ReplyWithJSONError(responseWriter, request, "No tags assigned to image", UserName, http.StatusNotFound)
				return
			}
			ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
			return
		}

		ReplyWithJSON(responseWriter, request, ImageTagGetResult{Tags: tags, ResultCount: len(tags)}, UserName)
		return
	}
	ReplyWithJSONError(responseWriter, request, "Please specify ImageID", UserName, http.StatusBadRequest)

}

//ImageTagsDeleteAPIRouter serves delete requests to /api/Image/{ImageID}/Tags/{TagID}
func ImageTagsDeleteAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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
	requestedTagID := urlVariables["TagID"]
	if requestedID != "" && requestedTagID != "" {
		//Grab specific image by ID
		parsedID, err := strconv.ParseUint(requestedID, 10, 32)
		if err != nil {
			ReplyWithJSONError(responseWriter, request, "ImageID could not be parsed into a number", UserName, http.StatusBadRequest)
			return
		}
		//Parse tag id
		parsedTagID, err := strconv.ParseUint(requestedTagID, 10, 32)
		if err != nil {
			ReplyWithJSONError(responseWriter, request, "TagID could not be parsed into a number", UserName, http.StatusBadRequest)
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
		if interfaces.UserPermission(permissions).HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || imageInfo.UploaderID != UserID) {
			ReplyWithJSONError(responseWriter, request, "You do not have permission to delete that", UserName, http.StatusForbidden)
			go routers.WriteAuditLogByName(UserName, "DELETE-IMAGETAG", UserName+" failed to delete image-tag with API. Insufficient permissions. "+requestedID+", "+requestedTagID)
			return
		}

		//Delete tag
		//Permission validated, now delete (ImageTags and Images)
		if err := database.DBInterface.RemoveTag(parsedTagID, parsedID); err != nil {
			ReplyWithJSONError(responseWriter, request, "Interal Database Error", UserName, http.StatusInternalServerError)
			go routers.WriteAuditLogByName(UserName, "DELETE-IMAGE", UserName+" failed to delete image tag with API. "+requestedID+", "+requestedTagID+", "+err.Error())
			return //Cancel delete
		}
		go routers.WriteAuditLogByName(UserName, "DELETE-IMAGETAG", UserName+" deleted image tag with API. "+requestedID)
		//Reply Success
		ReplyWithJSON(responseWriter, request, GenericResponse{Result: "Successfully deleted image tag " + requestedID + "-" + requestedTagID}, UserName)
		return
	}
	ReplyWithJSONError(responseWriter, request, "Please specify ImageID and TagID", UserName, http.StatusBadRequest)
}

type postImageTagInput struct {
	Tags string
}

//ImageTagsPostAPIRouter serves post requests to /api/Image/{ImageID}/Tags
func ImageTagsPostAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
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

		//Verify user can modify image tags
		if interfaces.UserPermission(permissions).HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || imageInfo.UploaderID != UserID) {
			go routers.WriteAuditLog(UserID, "ADD-IMAGETAG", UserName+" failed to add image tag. No permissions.")
			ReplyWithJSONError(responseWriter, request, "Insufficient permissions to add tag", UserName, http.StatusForbidden)
			return
		}

		//Parse user upload JSON request
		decoder := json.NewDecoder(request.Body)
		var uploadData postImageTagInput
		if err := decoder.Decode(&uploadData); err != nil {
			ReplyWithJSONError(responseWriter, request, "Failed to parse request data", "", http.StatusBadRequest)
			return
		}

		///////////////////
		//Get tags
		var validatedUserTags []uint64 //Will contain tags the user is allowed to use
		tagIDString := ""
		userQTags, err := database.DBInterface.GetQueryTags(uploadData.Tags, false)
		if err != nil {
			ReplyWithJSONError(responseWriter, request, "Database error parsing tags", UserName, http.StatusInternalServerError)
			return
		}

		warnings := ""

		for _, tag := range userQTags {
			if tag.Exists && tag.IsMeta == false {
				//Assign pre-existing tag
				//Permissions to tag validated above
				validatedUserTags = append(validatedUserTags, tag.ID)
				tagIDString = tagIDString + ", " + strconv.FormatUint(tag.ID, 10)
			} else if tag.IsMeta == false {
				//Create Tag
				//Validate permissions to create tags
				if interfaces.UserPermission(permissions).HasPermission(interfaces.AddTags) != true {
					go routers.WriteAuditLog(UserID, "CREATE-TAG", UserName+" failed to create tag ("+tag.Name+"). No permissions.")
					warnings += "Unable to use tag " + tag.Name + " due to insufficient permissions of user to create tags. "
					// /ValidatePermission
				} else {
					tagID, err := database.DBInterface.NewTag(tag.Name, tag.Description, UserID)
					if err != nil {
						go routers.WriteAuditLog(UserID, "CREATE-TAG", UserName+" failed to create tag ("+tag.Name+"). No database error. "+err.Error())
						warnings += "Unable to use tag (" + tag.Name + ") due to a database error. "
					} else {
						go routers.WriteAuditLog(UserID, "CREATE-TAG", UserName+" created a new tag. "+tag.Name)
						validatedUserTags = append(validatedUserTags, tagID)
						tagIDString = tagIDString + ", " + strconv.FormatUint(tagID, 10)
					}
				}
			}
		}
		///////////////////
		if err := database.DBInterface.AddTag(validatedUserTags, parsedID, UserID); err != nil {
			warnings += "Failed to add tag due to database error. "
			go routers.WriteAuditLog(UserID, "ADD-IMAGETAG", UserName+" failed to add tags ("+tagIDString+") to image. "+err.Error())
			uploadReply := uploadFileReply{LastID: parsedID, Errors: warnings}
			ReplyWithJSONStatus(responseWriter, request, uploadReply, UserName, http.StatusInternalServerError)
			return
		}

		//Send request to HandleImageUploadRequest
		uploadReply := uploadFileReply{LastID: parsedID, Errors: warnings}
		ReplyWithJSON(responseWriter, request, uploadReply, UserName)
	}
}

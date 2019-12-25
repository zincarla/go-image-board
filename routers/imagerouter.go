package routers

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers/templatecache"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//ImageRouter serves requests to /image
func ImageRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}
	var requestedID uint64
	var err error
	//If we are just now uploading the file, then we need to get ID from upload function
	switch request.FormValue("command") {
	case "uploadFile":
		if TemplateInput.UserName == "" {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to upload images"), 302)
			return
		}
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "INFO", []string{"Attempting to upload file"})
		requestedID, err = handleImageUpload(request, TemplateInput.UserName)
		if err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARNING", []string{err.Error()})
			TemplateInput.Message = "One or more warnings generated during upload. " + err.Error()
		}
		//redirect to a GET form of page
		http.Redirect(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		return
	case "ChangeVote":
		sImageID := request.FormValue("ID")
		if TemplateInput.UserName == "" || TemplateInput.UserID == 0 {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to vote on images"), 302)
			return
		}
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "INFO", []string{"Attempting to vote on image"})

		requestedID, err = strconv.ParseUint(sImageID, 10, 64)
		if err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARN", []string{"Failed to parse imageid to vote on"})
			TemplateInput.Message += "Failed to parse image id to vote on. "
			break
		}
		//Validate permission to vote
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.Message += "Failed to get image information. "
			break
		}

		if !(TemplateInput.UserPermissions.HasPermission(interfaces.ScoreImage) || (imageInfo.UploaderID == TemplateInput.UserID && config.Configuration.UsersControlOwnObjects)) {
			go WriteAuditLog(TemplateInput.UserID, "IMAGE-SCORE", TemplateInput.UserName+" failed to score image. No permissions.")
			TemplateInput.Message += "You do not have permissions to vote on this image. "
			break
		}
		// /ValidatePermission

		//At this point, user is validated
		Score, err := strconv.ParseInt(request.FormValue("NewVote"), 10, 64)
		if err != nil {
			TemplateInput.Message += "Failed to parse your vote value. "
			break
		}
		if Score < -10 || Score > 10 {
			TemplateInput.Message += "Score must be between -10 and 10"
			break
		}
		if err := database.DBInterface.UpdateUserVoteScore(TemplateInput.UserID, requestedID, Score); err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARN", []string{"Failed to set vote in database", err.Error()})
			TemplateInput.Message += "Failed to set vote in database, internal error. "
			break
		}
		TemplateInput.Message += "Successfully changed vote! "
	case "ChangeSource":
		sImageID := request.FormValue("ID")
		if TemplateInput.UserName == "" || TemplateInput.UserID == 0 {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to vote on images"), 302)
			return
		}
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "INFO", []string{"Attempting to source an image"})

		requestedID, err = strconv.ParseUint(sImageID, 10, 64)
		if err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARN", []string{"Failed to parse imageid to vote on"})
			TemplateInput.Message += "Failed to parse image id to vote on. "
			break
		}
		//Validate permission to vote
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.Message += "Failed to get image information. "
			break
		}

		if !(TemplateInput.UserPermissions.HasPermission(interfaces.SourceImage) || (imageInfo.UploaderID == TemplateInput.UserID && config.Configuration.UsersControlOwnObjects)) {
			go WriteAuditLog(TemplateInput.UserID, "IMAGE-SOURCE", TemplateInput.UserName+" failed to source image. No permissions.")
			TemplateInput.Message += "You do not have permissions to change the source of this image. "
			break
		}
		// /ValidatePermission

		//At this point, user is validated
		Source := request.FormValue("NewSource")

		if err := database.DBInterface.SetImageSource(requestedID, Source); err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARN", []string{"Failed to set source in database", err.Error()})
			TemplateInput.Message += "Failed to set source in database, internal error. "
			break
		}
		TemplateInput.Message += "Successfully changed source! "
	case "ChangeName":
		sImageID := request.FormValue("ID")
		if TemplateInput.UserName == "" || TemplateInput.UserID == 0 {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to vote on images"), 302)
			return
		}
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "INFO", []string{"Attempting to name an image"})

		requestedID, err = strconv.ParseUint(sImageID, 10, 64)
		if err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARN", []string{"Failed to parse imageid to change name on"})
			TemplateInput.Message += "Failed to parse image id. "
			break
		}
		//Validate permission to vote
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.Message += "Failed to get image information. "
			break
		}

		if !(TemplateInput.UserPermissions.HasPermission(interfaces.SourceImage) || (imageInfo.UploaderID == TemplateInput.UserID && config.Configuration.UsersControlOwnObjects)) {
			go WriteAuditLog(TemplateInput.UserID, "IMAGE-NAME", TemplateInput.UserName+" failed to name image. No permissions.")
			TemplateInput.Message += "You do not have permissions to change the name/description of this image. "
			break
		}
		// /ValidatePermission

		//At this point, user is validated
		Name := request.FormValue("NewName")
		Description := request.FormValue("NewDescription")

		if err := database.DBInterface.UpdateImage(requestedID, Name, Description, nil, nil, nil, nil); err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARN", []string{"Failed to set name in database", err.Error()})
			TemplateInput.Message += "Failed to set name/description in database, internal error. "
			break
		}
		TemplateInput.Message += "Successfully changed name/description! "
	case "RemoveTag":
		ImageID := request.FormValue("ID")
		TagID := request.FormValue("TagID")
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			break
		}
		if ImageID == "" || TagID == "" {
			TemplateInput.Message += "No ID provided to remove."
			break
		}

		iImageID, err := strconv.ParseUint(ImageID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id or image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse tag or image id ", err.Error()})
			break
		}
		requestedID = iImageID

		imageInfo, err := database.DBInterface.GetImage(iImageID)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}

		//Validate permission to upload
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != imageInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags on images. "
			go WriteAuditLogByName(TemplateInput.UserName, "REMOVE-IMAGETAG", TemplateInput.UserName+" failed to remove tag from image "+ImageID+". Insufficient permissions. "+TagID)
			break
		}
		// /ValidatePermission
		iID, err := strconv.ParseUint(TagID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id or image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse tag or image id ", err.Error()})
			break
		}
		//Remove tag
		if err := database.DBInterface.RemoveTag(iID, iImageID); err != nil {
			TemplateInput.Message += "Failed to remove tag. Was it attached in the first place?"
		} else {
			TemplateInput.Message += "Tag removed successfully"
			go WriteAuditLogByName(TemplateInput.UserName, "REMOVE-IMAGETAG", TemplateInput.UserName+" removed tag from image "+ImageID+". tag "+TagID)
		}

	case "AddTags":
		ImageID := request.FormValue("ID")
		userQuery := request.FormValue("NewTags")
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			break
		}
		//Translate UserID
		userID, err := database.DBInterface.GetUserID(TemplateInput.UserName)
		if err != nil {
			logging.LogInterface.WriteLog("TagRouter", "AddTags", TemplateInput.UserName, "ERROR", []string{"Could not get valid user id", err.Error()})
			TemplateInput.Message += "You muse be logged in to perform that action"
			break
		}
		if ImageID == "" {
			//redirect to images
			TemplateInput.Message += "Error parsing image id"
			break
		}

		iImageID, err := strconv.ParseUint(ImageID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}
		requestedID = iImageID
		imageInfo, err := database.DBInterface.GetImage(iImageID)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}
		//Validate permission to modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != imageInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags on images. "
			go WriteAuditLogByName(TemplateInput.UserName, "ADD-IMAGETAG", TemplateInput.UserName+" failed to add tag to image "+ImageID+". Insufficient permissions. "+userQuery)
			break
		}
		// /ValidatePermission

		///////////////////
		//Get tags
		var validatedUserTags []uint64 //Will contain tags the user is allowed to use
		tagIDString := ""
		userQTags, err := database.DBInterface.GetQueryTags(request.FormValue("NewTags"), false)
		if err != nil {
			TemplateInput.Message += "Failed to get tags from input"
			break
		}
		for _, tag := range userQTags {
			if tag.Exists && tag.IsMeta == false {
				//Assign pre-existing tag
				//Permissions to tag validated above
				validatedUserTags = append(validatedUserTags, tag.ID)
				tagIDString = tagIDString + ", " + strconv.FormatUint(tag.ID, 10)
			} else if tag.IsMeta == false {
				//Create Tag
				//Validate permissions to create tags
				if TemplateInput.UserPermissions.HasPermission(interfaces.AddTags) != true {
					logging.LogInterface.WriteLog("TagsRouter", "TagRouter", TemplateInput.UserName, "ERROR", []string{"Does not have create tag permission"})
					TemplateInput.Message += "Unable to use tag " + tag.Name + " due to insufficient permissions of user to create tags. "
					// /ValidatePermission
				} else {
					tagID, err := database.DBInterface.NewTag(tag.Name, tag.Description, userID)
					if err != nil {
						logging.LogInterface.WriteLog("TagsRouter", "TagRouter", TemplateInput.UserName, "WARNING", []string{"error attempting to create tag", err.Error(), tag.Name})
						TemplateInput.Message += "Unable to use tag " + tag.Name + " due to a database error. "
					} else {
						go WriteAuditLog(userID, "CREATE-TAG", TemplateInput.UserName+" created a new tag. "+tag.Name)
						validatedUserTags = append(validatedUserTags, tagID)
						tagIDString = tagIDString + ", " + strconv.FormatUint(tagID, 10)
					}
				}
			}
		}
		///////////////////
		if err := database.DBInterface.AddTag(validatedUserTags, iImageID, userID); err != nil {
			TemplateInput.Message += "Failed to add tag due to database error"
			logging.LogInterface.WriteLog("TagRouter", "AddTags", TemplateInput.UserName, "WARNING", []string{"error attempting to add tags to file", err.Error(), strconv.FormatUint(iImageID, 10), tagIDString})
		}
	case "ChangeRating":
		ImageID := request.FormValue("ID")
		newRating := strings.ToLower(request.FormValue("NewRating"))
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			break
		}

		if ImageID == "" {
			//redirect to images
			TemplateInput.Message += "Error parsing image id"
			break
		}

		iImageID, err := strconv.ParseUint(ImageID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}
		requestedID = iImageID
		imageInfo, err := database.DBInterface.GetImage(iImageID)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}
		//Validate permission to modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != imageInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags on images. "
			go WriteAuditLogByName(TemplateInput.UserName, "ADD-IMAGERATING", TemplateInput.UserName+" failed to edit rating for image "+ImageID+". Insufficient permissions. "+newRating)
			break
		}
		// /ValidatePermission
		//Change Rating

		if err = database.DBInterface.SetImageRating(iImageID, newRating); err != nil {
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to change image rating ", err.Error()})
			TemplateInput.Message += "Failed to change image rating, internal error ocurred. "
		}
	default:
		//Otherwise ID should come from request
		parsedValue, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			//No ID? Respond with blank template.
			TemplateInput.Message += "No image selected. "
			replyWithTemplate("image.html", TemplateInput, responseWriter)
			return
		}
		requestedID = parsedValue
	}

	//Get Imageinformation
	imageInfo, err := database.DBInterface.GetImage(requestedID)
	if err != nil {
		TemplateInput.Message += "Failed to get image information. "
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", "*", "ERROR", []string{"Failed to get image info for", strconv.FormatUint(requestedID, 10), err.Error()})
		replyWithTemplate("image.html", TemplateInput, responseWriter)
		return
	}

	//Get Collection Info
	imageInfo.MemberCollections, err = database.DBInterface.GetCollectionsWithImage(requestedID)
	if err != nil {
		//log err but no need to inform user
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", "*", "ERROR", []string{"Failed to get collection info for", strconv.FormatUint(requestedID, 10), err.Error()})
	}

	if TemplateInput.OldQuery != "" {
		//Get next and previous image based on query

		userQTags, err := database.DBInterface.GetQueryTags(TemplateInput.OldQuery, false)
		if err == nil {
			//if signed in, add user's global filters to query
			if TemplateInput.UserName != "" {
				userFilterTags, err := database.DBInterface.GetUserFilterTags(TemplateInput.UserID, false)
				if err != nil {
					logging.LogInterface.WriteLog("ImageRouter", "ImageQueryRouter", TemplateInput.UserName, "ERROR", []string{"Failed to load user's filter", err.Error()})
					TemplateInput.Message += "Failed to add your global filter to this query. Internal error. "
				} else {
					userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
				}
			}
			prevNextImage, err := database.DBInterface.GetPrevNexImages(userQTags, requestedID)
			if err == nil {
				if len(prevNextImage) == 2 {
					TemplateInput.NextMemberID = prevNextImage[1].ID
					TemplateInput.PreviousMemberID = prevNextImage[0].ID
				} else if len(prevNextImage) == 1 {
					if prevNextImage[0].ID > requestedID {
						TemplateInput.PreviousMemberID = prevNextImage[0].ID
					} else {
						TemplateInput.NextMemberID = prevNextImage[0].ID
					}
				}
			} else {
				logging.LogInterface.WriteLog("ImageRouter", "ImageQueryRouter", TemplateInput.UserName, "WARN", []string{"Failed to get next/prev image", err.Error()})
			}
		}
	}

	//Set Template with imageInfo
	TemplateInput.ImageContentInfo = imageInfo

	//Check is source is a url
	if _, err := url.ParseRequestURI(TemplateInput.ImageContentInfo.Source); err == nil {
		//Source is url
		TemplateInput.ImageContentInfo.SourceIsURL = true
	}

	//Get vote information
	//Validate permission to upload
	if TemplateInput.UserName != "" {
		TemplateInput.ImageContentInfo.UsersVotedScore, err = database.DBInterface.GetUserVoteScore(TemplateInput.UserID, requestedID)
	}

	//Get the image content information based on type (Img, vs video vs...)
	TemplateInput.ImageContent = templatecache.GetEmbedForContent(imageInfo.Location)

	TemplateInput.Tags, err = database.DBInterface.GetImageTags(imageInfo.ID)
	if err != nil {
		TemplateInput.Message += "Failed to load tags. "
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", "*", "ERROR", []string{"Failed to load tags", err.Error()})
	}

	replyWithTemplate("image.html", TemplateInput, responseWriter)
}

type uploadData struct {
	Name string
	ID   uint64
}

func handleImageUpload(request *http.Request, userName string) (uint64, error) {
	//Translate UserID
	userID, err := database.DBInterface.GetUserID(userName)
	if err != nil {
		go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. "+err.Error())
		return 0, errors.New("user not valid")
	}

	//Validate permission to upload
	userPermission, err := database.DBInterface.GetUserPermissionSet(userName)
	if err != nil {
		go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. "+err.Error())
		return 0, errors.New("Could not validate permission (SQL Error)")
	}

	//ParseCollection
	collectionName := strings.TrimSpace(request.FormValue("CollectionName"))
	//CacheCollectionInfo
	collectionInfo, err := database.DBInterface.GetCollectionByName(collectionName)
	if collectionName != "" && err != nil {
		//Want to add to collection, but the collection does not exist
		if interfaces.UserPermission(userPermission).HasPermission(interfaces.AddCollections) != true {
			go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. No permissions to create collection.")
			return 0, errors.New("User does not have create permission for collections")
		}
	} else if collectionName != "" && err == nil {
		//Want to add to a pre-existing collection
		if interfaces.UserPermission(userPermission).HasPermission(interfaces.ModifyCollections) != true &&
			(config.Configuration.UsersControlOwnObjects && collectionInfo.UploaderID != userID) {
			go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. No permissions to add members to collection.")
			return 0, errors.New("User does not have permission to update requested collection")
		}
	}

	if interfaces.UserPermission(userPermission).HasPermission(interfaces.UploadImage) != true {
		go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. No permissions.")
		return 0, errors.New("User does not have upload permission for images")
	}
	// /ValidatePermission

	errorCompilation := ""

	//Cache tags first, improves speed to calculate this once than for each image
	//Get tags
	var validatedUserTags []uint64 //Will contain tags the user is allowed to use
	tagIDString := ""
	userQTags, err := database.DBInterface.GetQueryTags(request.FormValue("SearchTags"), false)
	if err != nil {
		errorCompilation += "Failed to get tags from input"
	}
	for _, tag := range userQTags {
		if tag.Exists && tag.IsMeta == false {
			//Assign pre-existing tag
			//Validate permission to modify tags
			if interfaces.UserPermission(userPermission).HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true) {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Does not have modify tag permission"})
				errorCompilation += "Unable to use tag " + tag.Name + " due to insufficient permissions of user to tag images. "
				// /ValidatePermission
			} else {
				validatedUserTags = append(validatedUserTags, tag.ID)
				tagIDString = tagIDString + ", " + strconv.FormatUint(tag.ID, 10)
			}
		} else if tag.IsMeta == false {
			//Create Tag
			//Validate permissions to create tags
			if interfaces.UserPermission(userPermission).HasPermission(interfaces.AddTags) != true {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Does not have create tag permission"})
				errorCompilation += "Unable to use tag " + tag.Name + " due to insufficient permissions of user to create tags. "
				// /ValidatePermission
			} else {
				tagID, err := database.DBInterface.NewTag(tag.Name, tag.Description, userID)
				if err != nil {
					logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "WARNING", []string{"error attempting to create tag", err.Error(), tag.Name})
					errorCompilation += "Unable to use tag " + tag.Name + " due to a database error. "
				} else {
					go WriteAuditLog(userID, "CREATE-TAG", userName+" created a new tag. "+tag.Name)
					validatedUserTags = append(validatedUserTags, tagID)
					tagIDString = tagIDString + ", " + strconv.FormatUint(tagID, 10)
				}
			}
		}
	}

	var lastID uint64
	var uploadedIDs []uploadData
	request.ParseMultipartForm(config.Configuration.MaxUploadBytes)
	fileHeaders := request.MultipartForm.File["fileToUpload"]
	source := request.FormValue("Source")
	for _, fileHeader := range fileHeaders {
		switch ext := strings.ToLower(filepath.Ext(fileHeader.Filename)); ext {
		case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".svg", ".mpg", ".mov", ".webm", ".avi", ".mp4", ".mp3", ".ogg", ".wav", ".webp", ".tiff", ".tif":
			//Passes filter
		default:
			logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "WARN", []string{"Attempted to upload a file which did not pass filter", ext})
			errorCompilation += fileHeader.Filename + " is not a recognized file. "
			continue
		}
		fileStream, err := fileHeader.Open()
		if err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Upload image, could not open stream to save", err.Error()})
			errorCompilation += fileHeader.Filename + " could not be opened. "
		} else {
			originalName := fileHeader.Filename
			//Hash Image
			hashName, err := GetNewImageName(originalName, fileStream)
			if err != nil {
				errorCompilation += err.Error()
				fileStream.Close()
				continue
			}

			filePath := config.JoinPath(config.Configuration.ImageDirectory, hashName)
			//Check if file exists, if so, skip
			if _, err := os.Stat(filePath); err == nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "INFO", []string{"Skipping as file is already uploaded"})
				errorCompilation += fileHeader.Filename + " has already been uploaded. "
				fileStream.Close()
				continue
			}

			saveStream, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0660)
			if err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Upload image, failed to open new file", err.Error()})
				errorCompilation += fileHeader.Filename + " could not be saved, internal error. "
				saveStream.Close()
				fileStream.Close()
				continue
			}
			//Save Image
			_, err = fileStream.Seek(0, 0)
			if err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Upload image, failed to seek stream", err.Error()})
				errorCompilation += fileHeader.Filename + " could not be saved, internal error. "
				saveStream.Close()
				fileStream.Close()
				continue
			}
			io.Copy(saveStream, fileStream)
			saveStream.Close()
			//Add image to Database

			lastID, err = database.DBInterface.NewImage(hashName, hashName, userID, source)
			if err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"error attempting to add file to database", err.Error(), filePath})
				errorCompilation += fileHeader.Filename + " could not be added to database, internal error. "
				//Attempt to cleanup file
				if err := os.Remove(filePath); err != nil {
					logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"error attempting to remove orphaned file", err.Error(), filePath})
				}
				continue
			}

			uploadedIDs = append(uploadedIDs, uploadData{Name: originalName, ID: lastID})

			//Add tags
			if err := database.DBInterface.AddTag(validatedUserTags, lastID, userID); err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"failed to add tags", err.Error(), strconv.FormatUint(lastID, 10)})
				errorCompilation += "Failed to add tags to " + fileHeader.Filename + ". "
			} else {

				go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" tagged image "+strconv.FormatUint(lastID, 10)+" with "+tagIDString)
			}

			//Log success
			go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" successfully uploaded an image. "+strconv.FormatUint(lastID, 10))
			//Start go routine to generate thumbnail
			go GenerateThumbnail(hashName)
		}
		fileStream.Close()
	}
	//Now handle collection if requested

	if collectionName != "" {
		if collectionInfo.ID == 0 {
			collectionInfo.ID, err = database.DBInterface.NewCollection(collectionName, "", userID)
			if err != nil {
				errorCompilation += "Failed to create the collection requested, SQL error. "
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"error attempting to create collection", err.Error()})
			}
		}
		//If we had an error creating collection, this would still be 0, otherwise would have value or if collection already existed, would still have value other than 0
		if collectionInfo.ID != 0 {
			//Sort uploads by name
			sort.Slice(uploadedIDs, func(i, j int) bool {
				return uploadedIDs[i].Name < uploadedIDs[j].Name
			})
			var ids []uint64
			for _, v := range uploadedIDs {
				ids = append(ids, v.ID)
			}
			err = database.DBInterface.AddCollectionMember(collectionInfo.ID, ids, userID)
			if err != nil {
				errorCompilation += "Failed to add images to collection. "
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"error adding image to collection", err.Error()})
			}
		}
	}

	if errorCompilation != "" {
		return lastID, errors.New(errorCompilation)
	}
	return lastID, nil
}

//GetNewImageName uses the original filename and file contents to create a new name
func GetNewImageName(originalName string, fileStream io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, fileStream); err != nil {
		logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", "*", "ERROR", []string{"Error during hash", err.Error()})
		return "", errors.New(originalName + " could not be hashed. Internal error.")
	}

	return (fmt.Sprintf("%x", hasher.Sum(nil)) + filepath.Ext(originalName)), nil
}

//UploadFormRouter shows the upload form upon request
func UploadFormRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}
	replyWithTemplate("uploadform.html", TemplateInput, responseWriter)
}

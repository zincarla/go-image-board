package routers

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//ImageRouter serves requests to /image
func ImageRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)

	userQuery := TemplateInput.OldQuery

	var requestedID uint64
	var err error
	//If we are just now uploading the file, then we need to get ID from upload function
	switch {
	case request.FormValue("uploadFile") == "true":
		if TemplateInput.UserName == "" {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to upload images"), 302)
			return
		}
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "INFO", []string{"Attempting to upload file"})
		requestedID, err = handleImageUpload(request, userQuery, TemplateInput.UserName)
		if err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARNING", []string{err.Error()})
			TemplateInput.Message = "One or more warnings generated during upload. " + err.Error()
		}
	case request.FormValue("command") == "ChangeVote":
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
			go writeAuditLog(TemplateInput.UserID, "IMAGE-SCORE", TemplateInput.UserName+" failed to upload image. No permissions.")
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
		if err := database.DBInterface.UpdateUserVoteScore(TemplateInput.UserID, requestedID, Score); err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", TemplateInput.UserName, "WARN", []string{"Failed to set vote in database", err.Error()})
			TemplateInput.Message += "Failed to set vote in database, internal error. "
			break
		}
		TemplateInput.Message += "Successfully changed vote! "
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

	logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", "*", "INFO", []string{"Attempting to get file"})

	//Get Imageinformation
	imageInfo, err := database.DBInterface.GetImage(requestedID)
	if err != nil {
		TemplateInput.Message += "Failed to get image information. "
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", "*", "ERROR", []string{"Failed to get image info for", strconv.FormatUint(requestedID, 10)})
		replyWithTemplate("image.html", TemplateInput, responseWriter)
		return
	}
	//Set Template
	TemplateInput.ImageContentInfo = imageInfo

	//Get vote information
	//Validate permission to upload
	if TemplateInput.UserName != "" {
		TemplateInput.ImageContentInfo.UsersVotedScore, err = database.DBInterface.GetUserVoteScore(TemplateInput.UserID, requestedID)
	}

	//Get the image content information based on type (Img, vs video vs...)
	TemplateInput.ImageContent = getEmbedForContent(imageInfo.Location)

	TemplateInput.Tags, err = database.DBInterface.GetImageTags(imageInfo.ID)
	if err != nil {
		TemplateInput.Message += "Failed to load tags. "
		logging.LogInterface.WriteLog("ImageRouter", "ImageRouter", "*", "ERROR", []string{"Failed to load tags", err.Error()})
	}

	replyWithTemplate("image.html", TemplateInput, responseWriter)
}

//returns a mime given a file extension. This is only required for video and audio files so we can embed mime in video/audio element
func getMIME(extension string, fallback string) string {
	switch extension {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/avi"
	case ".mpg":
		return "video/mpeg"
	case ".mov":
		return "video/quicktime"
	case ".ogg":
		return "video/ogg"
	case ".mp3":
		return "audio/mpeg3"
	case ".wav":
		return "audio/wav"
	default:
		return fallback
	}
}

//Returns the html necessary to embed the specified file
func getEmbedForContent(imageLocation string) template.HTML {
	ToReturn := ""

	switch ext := filepath.Ext(strings.ToLower(imageLocation)); ext {
	case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".svg", ".webp":
		ToReturn = "<img src=\"/images/" + imageLocation + "\" alt=\"" + imageLocation + "\" />"
	case ".mpg", ".mov", ".webm", ".avi", ".mp4", ".mp3", ".ogg":
		ToReturn = "<video controls> <source src=\"/images/" + imageLocation + "\" type=\"" + getMIME(ext, "video/mp4") + "\">Your browser does not support the video tag.</video>"
	case ".wav":
		ToReturn = "<audio controls> <source src=\"/images/" + imageLocation + "\" type=\"" + getMIME(ext, "audio/wav") + "\">Your browser does not support the audio tag.</audio>"
	default:
		logging.LogInterface.WriteLog("ImageRouter", "getEmbedForConent", "*", "WARN", []string{"File uploaded, but did not match a filter during download", imageLocation})
		ToReturn = "<p>File format not supported. Click download.</p>"
	}

	return template.HTML(ToReturn)
}

func handleImageUpload(request *http.Request, userQuery string, userName string) (uint64, error) {
	//Translate UserID
	userID, err := database.DBInterface.GetUserID(userName)
	if err != nil {
		go writeAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. "+err.Error())
		return 0, errors.New("user not valid")
	}

	//Validate permission to upload
	userPermission, err := database.DBInterface.GetUserPermissionSet(userName)
	if err != nil {
		go writeAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. "+err.Error())
		return 0, errors.New("Could not validate permission (SQL Error)")
	}
	if interfaces.UserPermission(userPermission).HasPermission(interfaces.UploadImage) != true {
		go writeAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. No permissions.")
		return 0, errors.New("User does not have upload permission for images")
	}
	// /ValidatePermission

	errorCompilation := ""

	var lastID uint64
	request.ParseMultipartForm(config.Configuration.MaxUploadBytes)
	fileHeaders := request.MultipartForm.File["fileToUpload"]
	for _, fileHeader := range fileHeaders {
		switch ext := strings.ToLower(filepath.Ext(fileHeader.Filename)); ext {
		case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".svg", ".mpg", ".mov", ".webm", ".avi", ".mp4", ".mp3", ".ogg", ".wav", ".webp":
			//Passes filter
		default:
			logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "WARN", []string{"Attempted to upload a file which did not pass filter", ext})
			errorCompilation += fileHeader.Filename + " is not a recognized file. "
			continue
		}
		fileStream, err := fileHeader.Open()
		if err != nil {
			logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Upload image, could not open stream to save", err.Error()})
			errorCompilation += fileHeader.Filename + " could not be opened."
		} else {
			originalName := fileHeader.Filename
			//Hash Image
			hasher := sha256.New()
			if _, err := io.Copy(hasher, fileStream); err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Error during hash", err.Error()})
				errorCompilation += fileHeader.Filename + " could not be hashed. Internal error."
				fileStream.Close()
				continue
			}

			hashName := fmt.Sprintf("%x", hasher.Sum(nil)) + filepath.Ext(originalName)
			filePath := config.JoinPath(config.Configuration.ImageDirectory, hashName)
			//Check if file exists, if so, skip
			if _, err := os.Stat(filePath); err == nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "INFO", []string{"Skipping as file is already uploaded"})
				errorCompilation += fileHeader.Filename + " has already been uploaded."
				fileStream.Close()
				continue
			}

			saveStream, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0660)
			if err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Upload image, failed to open new file", err.Error()})
				errorCompilation += fileHeader.Filename + " could not be saved, internal error."
				saveStream.Close()
				fileStream.Close()
				continue
			}
			//Save Image
			_, err = fileStream.Seek(0, 0)
			if err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Upload image, failed to seek stream", err.Error()})
				errorCompilation += fileHeader.Filename + " could not be saved, internal error."
				saveStream.Close()
				fileStream.Close()
				continue
			}
			io.Copy(saveStream, fileStream)
			saveStream.Close()
			//Add image to Database

			lastID, err = database.DBInterface.NewImage(originalName, hashName, userID)
			if err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"error attempting to add file to database", err.Error(), filePath})
				errorCompilation += fileHeader.Filename + " could not be added to database, internal error."
				//Attempt to cleanup file
				if err := os.Remove(filePath); err != nil {
					logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"error attempting to remove orphaned file", err.Error(), filePath})
				}
				continue
			}
			//Log success
			go writeAuditLog(userID, "IMAGE-UPLOAD", userName+" successfully uploaded an image. "+strconv.FormatUint(lastID, 10))
			//Start go routine to generate thumbnail
			go GenerateThumbnail(hashName)

			//Get tags
			userQTags, err := database.DBInterface.GetQueryTags(userQuery)
			for _, tag := range userQTags {
				if tag.Exists && tag.IsMeta == false {
					//Assign pre-existing tag
					//Validate permission to modify tags
					if interfaces.UserPermission(userPermission).HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true) {
						logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Does not have modify tag permission"})
						errorCompilation += fileHeader.Filename + " could not be tagged with " + tag.Name + " due to insufficient permissions of user to tag."
						// /ValidatePermission
					} else {
						if err := database.DBInterface.AddTag(tag.ID, lastID, userID); err != nil {
							logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "WARNING", []string{"error attempting to add tag to new file", err.Error(), strconv.FormatUint(lastID, 10), strconv.FormatUint(tag.ID, 10)})
							errorCompilation += fileHeader.Filename + " could not be tagged with " + tag.Name + "."
						} else {
							go writeAuditLog(userID, "TAG-IMAGE", userName+" tagged an image. "+tag.Name+","+strconv.FormatUint(lastID, 10))
						}
					}
				} else if tag.IsMeta == false {
					//Create Tag
					//Validate permissions to create tags
					if interfaces.UserPermission(userPermission).HasPermission(interfaces.AddTags) != true {
						logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "ERROR", []string{"Does not have create tag permission"})
						errorCompilation += fileHeader.Filename + " could not be tagged with " + tag.Name + " due to insufficient permissions of user to create tags."
						// /ValidatePermission
					} else {
						tagID, err := database.DBInterface.NewTag(tag.Name, tag.Description, userID)
						if err != nil {
							logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "WARNING", []string{"error attempting to create tag for new file", err.Error(), strconv.FormatUint(lastID, 10), tag.Name})
							errorCompilation += fileHeader.Filename + " could not be tagged with " + tag.Name + "."
						} else {
							go writeAuditLog(userID, "CREATE-TAG", userName+" created a new tag. "+tag.Name)
							if err := database.DBInterface.AddTag(tagID, lastID, userID); err != nil {
								logging.LogInterface.WriteLog("ImageRouter", "handleImageUpload", userName, "WARNING", []string{"error attempting to add newly created tag to new file", err.Error(), strconv.FormatUint(lastID, 10), strconv.FormatUint(tagID, 10)})
								errorCompilation += fileHeader.Filename + " could not be tagged with " + tag.Name + "."
							} else {
								go writeAuditLog(userID, "TAG-IMAGE", userName+" tagged an image. "+tag.Name+","+strconv.FormatUint(lastID, 10))
							}
						}
					}
				}
			}
		}
		fileStream.Close()
	}
	if errorCompilation != "" {
		return lastID, errors.New(errorCompilation)
	}
	return lastID, nil
}

//UploadFormRouter shows the upload form upon request
func UploadFormRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)

	replyWithTemplate("uploadform.html", TemplateInput, responseWriter)
}

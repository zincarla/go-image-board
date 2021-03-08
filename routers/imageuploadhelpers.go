package routers

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type uploadData struct {
	Name string
	ID   uint64
}

func handleImageUpload(request *http.Request, userName string) (uint64, map[string]uint64, error) {
	//Translate UserID
	userID, err := database.DBInterface.GetUserID(userName)
	if err != nil {
		go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. "+err.Error())
		return 0, nil, errors.New("user not valid")
	}

	//Validate permission to upload
	userPermission, err := database.DBInterface.GetUserPermissionSet(userName)
	if err != nil {
		go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. "+err.Error())
		return 0, nil, errors.New("Could not validate permission (SQL Error)")
	}

	//ParseCollection
	collectionName := strings.TrimSpace(request.FormValue("CollectionName"))
	//CacheCollectionInfo
	collectionInfo, err := database.DBInterface.GetCollectionByName(collectionName)
	if collectionName != "" && err != nil {
		//Want to add to collection, but the collection does not exist
		if interfaces.UserPermission(userPermission).HasPermission(interfaces.AddCollections) != true {
			go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. No permissions to create collection.")
			return 0, nil, errors.New("User does not have create permission for collections")
		}
	} else if collectionName != "" && err == nil {
		//Want to add to a pre-existing collection
		if interfaces.UserPermission(userPermission).HasPermission(interfaces.ModifyCollections) != true &&
			(config.Configuration.UsersControlOwnObjects && collectionInfo.UploaderID != userID) {
			go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. No permissions to add members to collection.")
			return 0, nil, errors.New("User does not have permission to update requested collection")
		}
	}

	if interfaces.UserPermission(userPermission).HasPermission(interfaces.UploadImage) != true {
		go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" failed to upload image. No permissions.")
		return 0, nil, errors.New("User does not have upload permission for images")
	}
	// /ValidatePermission

	errorCompilation := ""
	duplicateIDs := make(map[string]uint64)

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
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"Does not have modify tag permission"})
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
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"Does not have create tag permission"})
				errorCompilation += "Unable to use tag " + tag.Name + " due to insufficient permissions of user to create tags. "
				// /ValidatePermission
			} else {
				tagID, err := database.DBInterface.NewTag(tag.Name, tag.Description, userID)
				if err != nil {
					logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"error attempting to create tag", err.Error(), tag.Name})
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
		case ".jpg", ".jpeg", ".jfif", ".bmp", ".gif", ".png", ".svg", ".mpg", ".mov", ".webm", ".avi", ".mp4", ".mp3", ".ogg", ".wav", ".webp", ".tiff", ".tif":
			//Passes filter
		default:
			logging.WriteLog(logging.LogLevelVerbose, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"Attempted to upload a file which did not pass filter", ext})
			errorCompilation += fileHeader.Filename + " is not a recognized file. "
			continue
		}
		fileStream, err := fileHeader.Open()
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"Upload image, could not open stream to save", err.Error()})
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

			filePath := path.Join(config.Configuration.ImageDirectory, hashName)
			//Check if file exists, if so, skip
			if _, err := os.Stat(filePath); err == nil {
				var duplicateID uint64
				dupInfo, ierr := database.DBInterface.GetImageByFileName(hashName)
				if ierr == nil {
					duplicateID = dupInfo.ID
				}
				logging.WriteLog(logging.LogLevelInfo, "imagerouter/handleImageUpload", userName, logging.ResultInfo, []string{"Skipping as file is already uploaded", fileHeader.Filename, filePath, strconv.FormatUint(duplicateID, 10)})
				if ierr == nil {
					//errorCompilation += fileHeader.Filename + " has already been uploaded as ID " + strconv.FormatUint(duplicateID, 10) + ". "
					duplicateIDs[fileHeader.Filename] = duplicateID
				} else {
					errorCompilation += fileHeader.Filename + " has already been uploaded. "
				}
				fileStream.Close()
				continue
			}

			saveStream, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0660)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"Upload image, failed to open new file", err.Error()})
				errorCompilation += fileHeader.Filename + " could not be saved, internal error. "
				saveStream.Close()
				fileStream.Close()
				continue
			}
			//Save Image
			_, err = fileStream.Seek(0, 0)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"Upload image, failed to seek stream", err.Error()})
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
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"error attempting to add file to database", err.Error(), filePath})
				errorCompilation += fileHeader.Filename + " could not be added to database, internal error. "
				//Attempt to cleanup file
				if err := os.Remove(filePath); err != nil {
					logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"error attempting to remove orphaned file", err.Error(), filePath})
				}
				continue
			}

			uploadedIDs = append(uploadedIDs, uploadData{Name: originalName, ID: lastID})

			//Add tags
			if err := database.DBInterface.AddTag(validatedUserTags, lastID, userID); err != nil {
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"failed to add tags", err.Error(), strconv.FormatUint(lastID, 10)})
				errorCompilation += "Failed to add tags to " + fileHeader.Filename + ". "
			} else {

				go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" tagged image "+strconv.FormatUint(lastID, 10)+" with "+tagIDString)
			}

			//Log success
			go WriteAuditLog(userID, "IMAGE-UPLOAD", userName+" successfully uploaded an image. "+strconv.FormatUint(lastID, 10))
			//Start go routine to generate thumbnail
			go GenerateThumbnail(hashName)
			go GeneratedHash(hashName, lastID)
		}
		fileStream.Close()
	}
	//Now handle collection if requested

	if collectionName != "" {
		if collectionInfo.ID == 0 {
			collectionInfo.ID, err = database.DBInterface.NewCollection(collectionName, "", userID)
			if err != nil {
				errorCompilation += "Failed to create the collection requested, SQL error. "
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"error attempting to create collection", err.Error()})
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
				logging.WriteLog(logging.LogLevelError, "imagerouter/handleImageUpload", userName, logging.ResultFailure, []string{"error adding image to collection", err.Error()})
			}
		}
	}

	if errorCompilation != "" {
		return lastID, duplicateIDs, errors.New(errorCompilation)
	}
	return lastID, duplicateIDs, nil
}

//GetNewImageName uses the original filename and file contents to create a new name
func GetNewImageName(originalName string, fileStream io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, fileStream); err != nil {
		logging.WriteLog(logging.LogLevelError, "imagerouter/GetNewImageName", "0", logging.ResultFailure, []string{"Error during hash", err.Error()})
		return "", errors.New(originalName + " could not be hashed. Internal error.")
	}

	return (fmt.Sprintf("%x", hasher.Sum(nil)) + filepath.Ext(originalName)), nil
}

//UploadFormRouter shows the upload form upon request
func UploadFormRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(responseWriter, request)
	replyWithTemplate("uploadform.html", TemplateInput, responseWriter, request)
}

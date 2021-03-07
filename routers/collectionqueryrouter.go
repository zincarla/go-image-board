package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
)

//CollectionsRouter serves requests to /collections
func CollectionsRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(responseWriter, request)
	TemplateInput.TotalResults = 0
	switch cmd := request.FormValue("command"); cmd {
	case "delete":
		if TemplateInput.UserInformation.Name == "" {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to delete a collection", "LogonRequired")
			return
		}

		//Get Collection ID
		parsedCollectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.Message += "Failed to get collection with that ID."
			break
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(parsedCollectionID)
		if err != nil {
			TemplateInput.Message += "Failed to delete collection. SQL Error. "
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveCollections) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.Message += "User does not have delete permission for collection. "
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. Insufficient permissions. "+request.FormValue("ID"))
			break
		}

		//Permission validated, now delete (Collection)
		if err := database.DBInterface.DeleteCollection(parsedCollectionID); err != nil {
			TemplateInput.Message += "Failed to delete collection. SQL Error. "
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" deleted collection. "+request.FormValue("ID")+", "+CollectionInfo.Name)
		TemplateInput.Message += "Successfully deleted collection " + CollectionInfo.Name + ". "
		redirectWithFlash(responseWriter, request, "/collections?"+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "Success")
		return
	case "deleteandmembers":
		if TemplateInput.UserInformation.Name == "" {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to delete a collection", "LogonRequired")
			return
		}

		//Get Collection ID
		parsedCollectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.Message += "Failed to get collection with that ID."
			break
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(parsedCollectionID)
		if err != nil {
			TemplateInput.Message += "Failed to delete collection. SQL Error. "
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveCollections) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.Message += "User does not have delete permission for collection. "
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. Insufficient permissions. "+request.FormValue("ID"))
			break
		}

		//Grab list of images
		CollectionMembers, _, err := database.DBInterface.GetCollectionMembers(parsedCollectionID, 0, 0)
		if err != nil {
			TemplateInput.Message += "Failed to delete collection. SQL Error getting collection memebers. "
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Check permissions for all members
		canDelete := true
		for _, ImageInfo := range CollectionMembers {
			//Validate Permission to delete
			if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveImage) != true && (config.Configuration.UsersControlOwnObjects != true || ImageInfo.UploaderID != TemplateInput.UserInformation.ID) {
				TemplateInput.Message += "User does not have delete permission for image. "
				go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" failed to delete image. Insufficient permissions. "+request.FormValue("ID"))
				canDelete = false
				break
			}
		}
		if !canDelete {
			break
		}

		//Permission validated, now delete (Collection)
		if err := database.DBInterface.DeleteCollection(parsedCollectionID); err != nil {
			TemplateInput.Message += "Failed to delete collection. SQL Error. "
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Delete images
		for _, ImageInfo := range CollectionMembers {
			err = database.DBInterface.DeleteImage(ImageInfo.ID)
			if err != nil {
				TemplateInput.Message += "Failed to delete image. "
				go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" failed to delete image.")
			} else {
				//Delete Image from Disk
				go os.Remove(path.Join(config.Configuration.ImageDirectory, ImageInfo.Location))
				//Delete thumbnail from disk
				go os.Remove(path.Join(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+ImageInfo.Location+".png"))
			}
		}

		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" deleted collection. "+request.FormValue("ID")+", "+CollectionInfo.Name)
		TemplateInput.Message += "Successfully deleted collection " + CollectionInfo.Name + ". "
		redirectWithFlash(responseWriter, request, "/collections?"+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "DeleteSuccess")
		return
	}

	//Get the page offset
	pageStartS := request.FormValue("PageStart")
	upageStart, err := strconv.ParseUint(pageStartS, 10, 32)
	var pageStart uint64
	pageStride := config.Configuration.PageStride
	if err == nil {
		//default to 0 on err
		pageStart = upageStart
	}

	userQTags, err := database.DBInterface.GetQueryTags(TemplateInput.OldQuery, true)
	if err == nil {
		//if signed in, add user's global filters to query
		if TemplateInput.UserInformation.Name != "" {
			userFilterTags, err := database.DBInterface.GetUserFilterTags(TemplateInput.UserInformation.ID, true)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "collectionqueryrouter/CollectionsRouter", TemplateInput.UserInformation.Name, logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
				TemplateInput.Message += "Failed to add your global filter to this query. Internal error. "
			} else {
				userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
			}
		}

		//Perform Query
		collectionInfo, MaxCount, err := database.DBInterface.SearchCollections(userQTags, pageStart, pageStride)
		if err == nil {
			TemplateInput.CollectionInfoList = collectionInfo
			TemplateInput.TotalResults = MaxCount
		} else {
			parsed := ""
			for _, tag := range userQTags {
				if tag.Exclude {
					parsed += "-"
				}
				if !tag.Exists {
					parsed += "!"
				}
				parsed += tag.Name + " "
			}
			logging.WriteLog(logging.LogLevelError, "CollectionQueryRouter/CollectionsRouter", "", logging.ResultFailure, []string{"Failed to search images", TemplateInput.OldQuery, parsed, err.Error()})
		}
	} else {
		logging.WriteLog(logging.LogLevelError, "CollectionQueryRouter/CollectionsRouter", "", logging.ResultFailure, []string{"Failed to validate tags", TemplateInput.OldQuery, err.Error()})
	}

	TemplateInput.Tags = userQTags
	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), "/collections")

	replyWithTemplate("collections.html", TemplateInput, responseWriter, request)
}

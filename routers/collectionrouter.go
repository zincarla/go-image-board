package routers

import (
	"database/sql"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

//CollectionGetRouter serves get requests to /collection
func CollectionGetRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	userQuery := TemplateInput.OldQuery
	var collectionID uint64
	var err error

	//Change StremView if requested
	if request.FormValue("ViewMode") == "stream" {
		TemplateInput.ViewMode = "stream"
		_, _, session := getSessionInformation(request)
		session.Values["ViewMode"] = "stream"
		session.Save(request, responseWriter)
	} else if request.FormValue("ViewMode") == "slideshow" {
		TemplateInput.ViewMode = "slideshow"
		_, _, session := getSessionInformation(request)
		session.Values["ViewMode"] = "slideshow"
		session.Save(request, responseWriter)
	} else if request.FormValue("ViewMode") != "" { //default to grid on invalid modes
		TemplateInput.ViewMode = "grid"
		_, _, session := getSessionInformation(request)
		session.Values["ViewMode"] = "grid"
		session.Save(request, responseWriter)
	}

	//Get the page offset
	pageStartS := request.FormValue("PageStart")
	pageStart, _ := strconv.ParseUint(pageStartS, 10, 32) //Either parses fine, or is 0, both works
	pageStride := config.Configuration.PageStride

	//Get Collection ID
	collectionID, err = strconv.ParseUint(request.FormValue("ID"), 10, 32)
	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Failed to parse requested collection ID.<br>")
		redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionError")
		return
	}

	//getCollectionInfo
	collectionInfo, err := database.DBInterface.GetCollection(collectionID)
	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Failed to get the requested collection.<br>")
		logging.WriteLog(logging.LogLevelError, "collectionimagerouter/CollectionImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get collection", strconv.FormatUint(collectionID, 10), err.Error()})
		redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionError")
		return
	}

	TemplateInput.CollectionInfo = collectionInfo
	//Parse tag results for next query
	imageInfo, MaxCount, err := database.DBInterface.GetCollectionMembers(collectionID, pageStart, pageStride)
	if err == nil {
		TemplateInput.ImageInfo = imageInfo
		TemplateInput.TotalResults = MaxCount
		TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(userQuery)+"&ID="+strconv.FormatUint(collectionInfo.ID, 10), "/collection")
		TemplateInput.Tags, err = database.DBInterface.GetCollectionTags(collectionInfo.ID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get collection tags.<br>")
			logging.WriteLog(logging.LogLevelError, "collectionimagerouter/CollectionImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get collection tags", strconv.FormatUint(collectionID, 10), err.Error()})
		}
	} else {
		logging.WriteLog(logging.LogLevelError, "collectionimagerouter/CollectionImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get collection images", strconv.FormatUint(collectionID, 10), err.Error()})
		TemplateInput.HTMLMessage += template.HTML("Failed to get collection members.<br>")
	}

	replyWithTemplate("collection.html", TemplateInput, responseWriter, request)
}

//CollectionPostRouter serves post requests to /collection
func CollectionPostRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	userQuery := TemplateInput.OldQuery
	var collectionID uint64

	switch cmd := request.FormValue("command"); cmd {
	case "deletemember": //Remove a single image from a collection, and if last image, the collection itself
		if TemplateInput.UserInformation.Name == "" {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to remove collection members", "LogonRequired")
			return
		}

		//Get Collection ID
		collectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get collection with that ID.<br>")
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Get Collection ID
		parsedImageID, err := strconv.ParseUint(request.FormValue("ImageID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get image with that ID.<br>")
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to edit collection. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollectionMembers) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.HTMLMessage += template.HTML("You do not have edit member permission for collection.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to delete collection. Insufficient permissions. "+request.FormValue("ID"))
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Permission validated, now delete (CollectionMember)
		if CollectionInfo.Members <= 1 {
			if err := database.DBInterface.DeleteCollection(collectionID); err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to delete collection. SQL Error.<br>")
				go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to remove member from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+err.Error())
				redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
				return
			}
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" removed image from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+CollectionInfo.Name)
			TemplateInput.HTMLMessage += template.HTML("Successfully remove image from collection. Collection empty, so collection was also removed.<br>")
			//Redirect since we deleted collection
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "DeleteSuccess")
			return
		}
		if err := database.DBInterface.RemoveCollectionMember(collectionID, parsedImageID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete collection member. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to remove member from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" removed image from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+CollectionInfo.Name)
		TemplateInput.HTMLMessage += template.HTML("Successfully removed image from collection.<br>")
		redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionSuccess")
		return
	case "addcollectionmember": //Add image to a collection, and if collection does not exist, create it
		//Get Image ID
		parsedImageID, err := strconv.ParseUint(request.FormValue("ImageID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get image with that ID to add to collection.<br>")
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		collection, err := database.DBInterface.GetCollectionByName(request.FormValue("CollectionName"))
		if err != nil && err == sql.ErrNoRows {
			//Does not exist, so check if we can create
			//Validate Permission
			if TemplateInput.UserPermissions.HasPermission(interfaces.AddCollections) != true {
				TemplateInput.HTMLMessage += template.HTML("You do not have create collection permissions.<br>")
				go WriteAuditLogByName(TemplateInput.UserInformation.Name, "ADD-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to create a collection. Insufficient permissions. "+request.FormValue("CollectionName"))
				redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
				return
			}

			if len(request.FormValue("CollectionName")) < 3 || len(request.FormValue("CollectionName")) > 255 {
				TemplateInput.HTMLMessage += template.HTML("CollectionName should be longer than 3 characters but less than 255.<br>")
				redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
				return
			}

			collectionID, err = database.DBInterface.NewCollection(request.FormValue("CollectionName"), "", TemplateInput.UserInformation.ID)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "collectionimagerouter/CollectionImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"failed to create collection", err.Error()})
				TemplateInput.HTMLMessage += template.HTML("Failed to create new collection. SQL Error.<br>")
				redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
				return
			}

			TemplateInput.HTMLMessage += template.HTML("New collection created successfully.<br>")

			if err := database.DBInterface.AddCollectionMember(collectionID, append([]uint64{}, parsedImageID), TemplateInput.UserInformation.ID); err != nil {
				logging.WriteLog(logging.LogLevelError, "collectionimagerouter/CollectionImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"failed to add image to new collection", err.Error()})
				TemplateInput.HTMLMessage += template.HTML("Failed to add image to new collection. SQL Error.<br>")
				redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
				return
			}

			TemplateInput.HTMLMessage += template.HTML("Image added to new collection.<br>")

			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionSucceeded")
			return
		} else if err != nil {
			logging.WriteLog(logging.LogLevelError, "collectionimagerouter/CollectionImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"failed to query collection", err.Error()})
			TemplateInput.HTMLMessage += template.HTML("Failed to query collection.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Exists, see if we can add to it
		//Validate Permission
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollectionMembers) != true && (config.Configuration.UsersControlOwnObjects != true || collection.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.HTMLMessage += template.HTML("You do not have edit member permission for this collection.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "ADD-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to add a collection member. Insufficient permissions. "+collection.Name)
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Add image to collection
		if err := database.DBInterface.AddCollectionMember(collection.ID, append([]uint64{}, parsedImageID), TemplateInput.UserInformation.ID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to add image to collection. SQL error. Check if image is already part of the collection.<br>")
		}
		TemplateInput.HTMLMessage += template.HTML("Image added to collection.<br>")
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "CollectionSuccess")
		return
	case "modify": //Change a collection's name/description
		if TemplateInput.UserInformation.Name == "" {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to modify a collection", "LogonRequired")
			return
		}

		//Get Collection ID
		collectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get collection with that ID.<br>")
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Validate NewName
		newName := strings.TrimSpace(request.FormValue("NewName"))
		if len(newName) < 3 || len(newName) > 255 {
			TemplateInput.HTMLMessage += template.HTML("New Name is an unsupported length. Please ensure it is longer than 3 characters but shorter than 255.<br>")
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}
		//Validate NewDescription
		newDesc := strings.TrimSpace(request.FormValue("NewDescription"))
		if len(newDesc) > 255 {
			TemplateInput.HTMLMessage += template.HTML("Description cannot be longer than 255 characters.<br>")
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to edit collection. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-COLLECTION", TemplateInput.UserInformation.Name+" failed to modify collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Validate unique name
		newNameCollection, err := database.DBInterface.GetCollectionByName(newName)
		if err == nil && newNameCollection.ID != CollectionInfo.ID {
			TemplateInput.HTMLMessage += template.HTML(template.HTMLEscapeString(newName) + " is already in use, plese select a different name.<br>")
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollections) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.HTMLMessage += template.HTML("You do not have edit member permission for collection.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-COLLECTION", TemplateInput.UserInformation.Name+" failed to modify collection. Insufficient permissions. "+request.FormValue("ID"))
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Permission validated, now modify
		if err := database.DBInterface.UpdateCollection(collectionID, newName, newDesc); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to modify collection. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-COLLECTION", TemplateInput.UserInformation.Name+" failed to modify collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-COLLECTION", TemplateInput.UserInformation.Name+" modified collection ("+request.FormValue("ID")+")")
		TemplateInput.HTMLMessage += template.HTML("Successfully modified collection.<br>")
		redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionSuccess")
		return
	case "deletecollection":
		if !TemplateInput.IsLoggedOn() {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to delete a collection", "LogonRequired")
			return
		}

		//Get Collection ID
		collectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get collection with that ID.<br>")
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete collection. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveCollections) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.HTMLMessage += template.HTML("User does not have delete permission for collection.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. Insufficient permissions. "+request.FormValue("ID"))
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}

		//Permission validated, now delete (Collection)
		if err := database.DBInterface.DeleteCollection(collectionID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete collection. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionFailed")
			return
		}
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" deleted collection. "+request.FormValue("ID")+", "+CollectionInfo.Name)
		TemplateInput.HTMLMessage += template.HTML("Successfully deleted collection " + template.HTMLEscapeString(CollectionInfo.Name) + ".<br>")
		redirectWithFlash(responseWriter, request, "/collections?"+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "Success")
		return
	case "deletecollectionandmembers":
		if !TemplateInput.IsLoggedOn() {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to delete a collection", "LogonRequired")
			return
		}

		//Get Collection ID
		collectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get collection with that ID.<br>")
			redirectWithFlash(responseWriter, request, "/collections?&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete collection. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveCollections) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.HTMLMessage += template.HTML("User does not have delete permission for collection.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. Insufficient permissions. "+request.FormValue("ID"))
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Grab list of images
		CollectionMembers, _, err := database.DBInterface.GetCollectionMembers(collectionID, 0, 0)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete collection. SQL Error getting collection memebers.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Check permissions for all members
		canDelete := true
		for _, ImageInfo := range CollectionMembers {
			//Validate Permission to delete
			if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveImage) != true && (config.Configuration.UsersControlOwnObjects != true || ImageInfo.UploaderID != TemplateInput.UserInformation.ID) {
				TemplateInput.HTMLMessage += template.HTML("User does not have delete permission for image.<br>")
				go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" failed to delete image. Insufficient permissions. "+request.FormValue("ID"))
				canDelete = false
				break
			}
		}
		if !canDelete {
			TemplateInput.HTMLMessage += template.HTML("You do not have permission to delete all collection members.<br>")
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Permission validated, now delete (Collection)
		if err := database.DBInterface.DeleteCollection(collectionID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete collection. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/collection?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Delete images
		for _, ImageInfo := range CollectionMembers {
			err = database.DBInterface.DeleteImage(ImageInfo.ID)
			if err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to delete image " + strconv.FormatUint(ImageInfo.ID, 10) + ".<br>")
				go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" failed to delete image.")
			} else {
				//Delete Image from Disk
				go os.Remove(path.Join(config.Configuration.ImageDirectory, ImageInfo.Location))
				//Delete thumbnail from disk
				go os.Remove(path.Join(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+ImageInfo.Location+".png"))
			}
		}

		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-COLLECTION", TemplateInput.UserInformation.Name+" deleted collection. "+request.FormValue("ID")+", "+CollectionInfo.Name)
		TemplateInput.HTMLMessage += template.HTML("Successfully deleted collection " + template.HTMLEscapeString(CollectionInfo.Name) + ".<br>")
		redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteSuccess")
		return
	}

	TemplateInput.HTMLMessage += template.HTML("Invalid or unknown command.<br>")
	redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(userQuery), TemplateInput.HTMLMessage, "CollectionError")
}

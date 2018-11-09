package routers

import (
	"database/sql"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

//CollectionImageRouter serves requests to /collectionimages
func CollectionImageRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}
	userQuery := TemplateInput.OldQuery
	var collectionID uint64
	var err error

	//Change StremView if requested
	if request.FormValue("StreamViewPreference") == "true" {
		TemplateInput.StreamView = true
		_, _, session := getSessionInformation(request)
		session.Values["StreamView"] = "true"
		session.Save(request, responseWriter)
	} else if request.FormValue("StreamViewPreference") == "false" {
		TemplateInput.StreamView = false
		_, _, session := getSessionInformation(request)
		session.Values["StreamView"] = ""
		session.Save(request, responseWriter)
	}

	switch cmd := request.FormValue("command"); cmd {
	case "delete":
		if TemplateInput.UserName == "" {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to remove collection members"), 302)
			return
		}

		//Get Collection ID
		parsedCollectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.Message += "Failed to get collection with that ID."
			break
		}

		//Get Collection ID
		parsedImageID, err := strconv.ParseUint(request.FormValue("ImageID"), 10, 32)
		if err != nil {
			TemplateInput.Message += "Failed to get image with that ID."
			break
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(parsedCollectionID)
		if err != nil {
			TemplateInput.Message += "Failed to edit collection. SQL Error. "
			go writeAuditLogByName(TemplateInput.UserName, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserName+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollectionMembers) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserID) {
			TemplateInput.Message += "User does not have edit member permission for collection. "
			go writeAuditLogByName(TemplateInput.UserName, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserName+" failed to delete collection. Insufficient permissions. "+request.FormValue("ID"))
			break
		}

		//Permission validated, now delete (CollectionMember)
		if CollectionInfo.Members <= 1 {
			if err := database.DBInterface.DeleteCollection(parsedCollectionID); err != nil {
				TemplateInput.Message += "Failed to delete collection. SQL Error. "
				go writeAuditLogByName(TemplateInput.UserName, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserName+" failed to remove member from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+err.Error())
				break //Cancel remove
			}
			go writeAuditLogByName(TemplateInput.UserName, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserName+" removed image from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+CollectionInfo.Name)
			TemplateInput.Message += "Successfully remove image from collection. Collection empty, so collection was also removed. "
			//Redirect since we deleted collection
			http.Redirect(responseWriter, request, "/collections?prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		} else {
			if err := database.DBInterface.RemoveCollectionMember(parsedCollectionID, parsedImageID); err != nil {
				TemplateInput.Message += "Failed to delete collection member. SQL Error. "
				go writeAuditLogByName(TemplateInput.UserName, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserName+" failed to remove member from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+err.Error())
				break //Cancel remove
			}
			go writeAuditLogByName(TemplateInput.UserName, "REMOVE-COLLECTIONMEMBER", TemplateInput.UserName+" removed image from collection. "+request.FormValue("ImageID")+" from "+request.FormValue("ID")+", "+CollectionInfo.Name)
			TemplateInput.Message += "Successfully remove image from collection. "
		}
	case "addcollectionmember":
		//Get Image ID
		parsedImageID, err := strconv.ParseUint(request.FormValue("ImageID"), 10, 32)
		if err != nil {
			TemplateInput.Message += "Failed to get image with that ID to add to collection."
			break
		}

		collection, err := database.DBInterface.GetCollectionByName(request.FormValue("CollectionName"))
		if err != nil && err == sql.ErrNoRows {
			//Does not exist, so check if we can create
			//Validate Permission
			if TemplateInput.UserPermissions.HasPermission(interfaces.AddCollections) != true {
				TemplateInput.Message += "You do not have create collection permissions. "
				go writeAuditLogByName(TemplateInput.UserName, "ADD-COLLECTIONMEMBER", TemplateInput.UserName+" failed to create a collection. Insufficient permissions. "+request.FormValue("CollectionName"))
				break
			}

			if len(request.FormValue("CollectionName")) < 3 || len(request.FormValue("CollectionName")) > 255 {
				TemplateInput.Message += "CollectionName should be longer than 3 characters but less than 255. "
				break
			}

			collectionID, err = database.DBInterface.NewCollection(request.FormValue("CollectionName"), "", TemplateInput.UserID)
			if err != nil {
				logging.LogInterface.WriteLog("CollectionImageRouter", "CollectionImageRouter", TemplateInput.UserName, "ERROR", []string{"failed to create collection", err.Error()})
				TemplateInput.Message += "Failed to create new collection. SQL Error. "
				break
			}

			TemplateInput.Message += "New collection created successfully. "

			if err := database.DBInterface.AddCollectionMember(collectionID, append([]uint64{}, parsedImageID), TemplateInput.UserID); err != nil {
				logging.LogInterface.WriteLog("CollectionImageRouter", "CollectionImageRouter", TemplateInput.UserName, "ERROR", []string{"failed to add image to new collection", err.Error()})
				TemplateInput.Message += "Failed to add image to new collection. SQL Error. "
				break
			}

			TemplateInput.Message += "Image added to new collection. "

			break
		} else if err != nil {
			logging.LogInterface.WriteLog("CollectionImageRouter", "CollectionImageRouter", TemplateInput.UserName, "ERROR", []string{"failed to query collection", err.Error()})
			TemplateInput.Message += "Failed to query collection."
			break
		}

		collectionID = collection.ID //For fallthrough

		//Exists, see if we can add to it
		//Validate Permission
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollectionMembers) != true && (config.Configuration.UsersControlOwnObjects != true || collection.UploaderID != TemplateInput.UserID) {
			TemplateInput.Message += "You do not have edit member permission for this collection. "
			go writeAuditLogByName(TemplateInput.UserName, "ADD-COLLECTIONMEMBER", TemplateInput.UserName+" failed to add a collection member. Insufficient permissions. "+collection.Name)
			break
		}

		//Add image to collection
		if err := database.DBInterface.AddCollectionMember(collection.ID, append([]uint64{}, parsedImageID), TemplateInput.UserID); err != nil {
			TemplateInput.Message += "Failed to add image to collection. SQL error. Check if image is already part of the collection. "
		} else {
			TemplateInput.Message += "Image added to collection. "
		}
	case "modify":
		if TemplateInput.UserName == "" {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to modify a collection"), 302)
			return
		}

		//Get Collection ID
		collectionID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.Message += "Failed to get collection with that ID."
			break
		}

		//Validate NewName
		newName := strings.TrimSpace(request.FormValue("NewName"))
		if len(newName) < 3 || len(newName) > 255 {
			TemplateInput.Message += "New Name is an unsupported length. Please ensure it is longer than 3 characters but shorter than 255"
			break
		}
		//Validate NewDescription
		newDesc := strings.TrimSpace(request.FormValue("NewDescription"))
		if len(newDesc) > 255 {
			TemplateInput.Message += "Description cannot be longer than 255 characters"
			break
		}

		//Cache collection data
		CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
		if err != nil {
			TemplateInput.Message += "Failed to edit collection. SQL Error. "
			go writeAuditLogByName(TemplateInput.UserName, "MODIFY-COLLECTION", TemplateInput.UserName+" failed to modify collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Validate unique name
		newNameCollection, err := database.DBInterface.GetCollectionByName(newName)
		if err == nil && newNameCollection.ID != CollectionInfo.ID {
			TemplateInput.Message += newName + " is already in use, plese select a different name. "
			break
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollections) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserID) {
			TemplateInput.Message += "User does not have edit member permission for collection. "
			go writeAuditLogByName(TemplateInput.UserName, "MODIFY-COLLECTION", TemplateInput.UserName+" failed to modify collection. Insufficient permissions. "+request.FormValue("ID"))
			break
		}

		//Permission validated, now modify
		if err := database.DBInterface.UpdateCollection(collectionID, newName, newDesc); err != nil {
			TemplateInput.Message += "Failed to modify collection. SQL Error. "
			go writeAuditLogByName(TemplateInput.UserName, "MODIFY-COLLECTION", TemplateInput.UserName+" failed to modify collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel remove
		}
		go writeAuditLogByName(TemplateInput.UserName, "MODIFY-COLLECTION", TemplateInput.UserName+" modified collection ("+request.FormValue("ID")+")")
		TemplateInput.Message += "Successfully modified collection."
	}

	//Get the page offset
	pageStartS := request.FormValue("PageStart")
	pageStart, _ := strconv.ParseUint(pageStartS, 10, 32) //Either parses fine, or is 0, both works
	pageStride := config.Configuration.PageStride

	//Get Collection ID
	if collectionID == 0 {
		collectionID, err = strconv.ParseUint(request.FormValue("ID"), 10, 32)
	}
	if err != nil {
		TemplateInput.Message += "Failed to parse requested collection ID."
	} else {
		//getCollectionInfo
		collectionInfo, err := database.DBInterface.GetCollection(collectionID)
		if err != nil {
			TemplateInput.Message += "Failed to get the requested collection " + request.FormValue("ID")
			logging.LogInterface.WriteLog("CollectionRouter", "CollectionRouter", "*", "ERROR", []string{"Failed to get collection", strconv.FormatUint(collectionID, 10), err.Error()})

		} else {
			TemplateInput.CollectionInfo = collectionInfo
			//Parse tag results for next query
			imageInfo, MaxCount, err := database.DBInterface.GetCollectionMembers(collectionID, pageStart, pageStride)
			if err == nil {
				TemplateInput.ImageInfo = imageInfo
				TemplateInput.TotalResults = MaxCount
				TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(userQuery)+"&ID="+strconv.FormatUint(collectionInfo.ID, 10), "/collectionimages")
				TemplateInput.Tags, err = database.DBInterface.GetCollectionTags(collectionInfo.ID)
				if err != nil {
					TemplateInput.Message += "Failed to get collection tags."
					logging.LogInterface.WriteLog("CollectionRouter", "CollectionRouter", "*", "ERROR", []string{"Failed to get collection tags", strconv.FormatUint(collectionID, 10), err.Error()})
				}
			} else {
				logging.LogInterface.WriteLog("CollectionRouter", "CollectionRouter", "*", "ERROR", []string{"Failed to get collection images", strconv.FormatUint(collectionID, 10), err.Error()})
				TemplateInput.Message += "Failed to get collection members."
			}
		}
	}

	replyWithTemplate("collection.html", TemplateInput, responseWriter)
}

//CollectionImageOrderRouter serves requests to /collectionorder
func CollectionImageOrderRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}
	var collectionID uint64
	var err error

	if TemplateInput.UserName == "" {
		//Redirect to logon
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to modify collection members"), 302)
		return
	}

	//Get Collection ID
	collectionID, err = strconv.ParseUint(request.FormValue("ID"), 10, 32)
	if err != nil {
		TemplateInput.Message += "Failed to get collection with that ID. "
		http.Redirect(responseWriter, request, "/collections?prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		return
	}

	//Cache collection data
	CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
	if err != nil {
		TemplateInput.Message += "Failed to modify collection. SQL Error. "
		go writeAuditLogByName(TemplateInput.UserName, "MODIFY-COLLECTIONMEMBER", TemplateInput.UserName+" failed to modify collection. "+request.FormValue("ID")+", "+err.Error())
		http.Redirect(responseWriter, request, "/collections?prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		return
	}

	//Validate Permission to Modify
	if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollectionMembers) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserID) {
		TemplateInput.Message += "You do not have edit member permission for collection. "
		go writeAuditLogByName(TemplateInput.UserName, "MODIFY-COLLECTIONMEMBER", TemplateInput.UserName+" failed to modify collection order. Insufficient permissions. "+request.FormValue("ID"))
		http.Redirect(responseWriter, request, "/collections?prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		return
	}

	switch cmd := request.FormValue("command"); cmd {
	case "reorder":
		//Get and parse request.FormValue("NewOrder") //List of IDs
		var idList []uint64
		idStrings := strings.Split(request.FormValue("NewOrder"), ",")
		for _, v := range idStrings {
			//TryParse uint
			imageID, err := strconv.ParseUint(v, 10, 32)
			if err == nil {
				idList = append(idList, imageID)
			} else {
				//Just log for user
				TemplateInput.Message += v + " skipped. "
			}
		}

		//Loop through and send reorder commands
		if len(idList) > 0 {
			for i, v := range idList {
				if err := database.DBInterface.UpdateCollectionMember(collectionID, v, uint64(i)); err != nil {
					TemplateInput.Message += "Failed to reorder imageID " + strconv.FormatUint(v, 10) + " to " + strconv.Itoa(i)
				}
			}
		} else {
			TemplateInput.Message += "Reorder cancelled as image list empty after parsing. "
		}

		//redirect back to collection view with messages
		http.Redirect(responseWriter, request, "/collectionimages?ID="+request.FormValue("ID")+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
	}

	//getCollectionInfo
	collectionInfo, err := database.DBInterface.GetCollection(collectionID)
	if err != nil {
		TemplateInput.Message += "Failed to get the requested collection " + request.FormValue("ID")
		logging.LogInterface.WriteLog("CollectionRouter", "CollectionRouter", "*", "ERROR", []string{"Failed to get collection", strconv.FormatUint(collectionID, 10), err.Error()})

	} else {
		TemplateInput.CollectionInfo = collectionInfo
		//Parse tag results for next query
		imageInfo, MaxCount, err := database.DBInterface.GetCollectionMembers(collectionID, 0, 0)
		if err == nil {
			TemplateInput.ImageInfo = imageInfo
			TemplateInput.TotalResults = MaxCount
		} else {
			logging.LogInterface.WriteLog("CollectionRouter", "CollectionRouter", "*", "ERROR", []string{"Failed to get collection images", strconv.FormatUint(collectionID, 10), err.Error()})
			TemplateInput.Message += "Failed to get collection members."
		}
	}

	replyWithTemplate("collectionreorder.html", TemplateInput, responseWriter)
}

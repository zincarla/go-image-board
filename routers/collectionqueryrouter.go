package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"net/url"
	"strconv"
)

//CollectionsRouter serves requests to /collections
func CollectionsRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	TemplateInput.TotalResults = 0

	switch cmd := request.FormValue("command"); cmd {
	case "delete":
		if TemplateInput.UserName == "" {
			//Redirect to logon
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to delete collections"), 302)
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
			go writeAuditLogByName(TemplateInput.UserName, "DELETE-COLLECTION", TemplateInput.UserName+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveCollections) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserID) {
			TemplateInput.Message += "User does not have delete permission for collection. "
			go writeAuditLogByName(TemplateInput.UserName, "DELETE-COLLECTION", TemplateInput.UserName+" failed to delete collection. Insufficient permissions. "+request.FormValue("ID"))
			break
		}

		//Permission validated, now delete (Collection)
		if err := database.DBInterface.DeleteCollection(parsedCollectionID); err != nil {
			TemplateInput.Message += "Failed to delete collection. SQL Error. "
			go writeAuditLogByName(TemplateInput.UserName, "DELETE-COLLECTION", TemplateInput.UserName+" failed to delete collection. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}
		go writeAuditLogByName(TemplateInput.UserName, "DELETE-COLLECTION", TemplateInput.UserName+" deleted collection. "+request.FormValue("ID")+", "+CollectionInfo.Name)
		TemplateInput.Message += "Successfully deleted collection " + CollectionInfo.Name + ". "
		http.Redirect(responseWriter, request, "/collections?prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
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
		if TemplateInput.UserName != "" {
			userFilterTags, err := database.DBInterface.GetUserFilterTags(TemplateInput.UserID, true)
			if err != nil {
				logging.LogInterface.WriteLog("CollectionQueryRouter", "CollectionsRouter", TemplateInput.UserName, "ERROR", []string{"Failed to load user's filter", err.Error()})
				TemplateInput.Message += "Failed to add your global filter to this query. Internal error. "
			} else {
				userQTags = append(userQTags, userFilterTags...)
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
			logging.LogInterface.WriteLog("CollectionQueryRouter", "CollectionsRouter", "*", "ERROR", []string{"Failed to search images", TemplateInput.OldQuery, parsed, err.Error()})
		}
	} else {
		logging.LogInterface.WriteLog("CollectionQueryRouter", "CollectionsRouter", "*", "ERROR", []string{"Failed to validate tags", TemplateInput.OldQuery, err.Error()})
	}

	/*
		collectionInfo, MaxCount, err := database.DBInterface.GetCollections(pageStart, pageStride)
		if err != nil {
			logging.LogInterface.WriteLog("CollectionQueryRouter", "CollectionsRouter", TemplateInput.UserName, "ERROR", []string{"Error getting collection list", err.Error()})
			TemplateInput.Message += "Error getting collections."
		} else {
			TemplateInput.CollectionInfoList = collectionInfo
			TemplateInput.TotalResults = MaxCount
		}
	*/

	TemplateInput.Tags = userQTags
	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), "/collections")

	replyWithTemplate("collections.html", TemplateInput, responseWriter)
}

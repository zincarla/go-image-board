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

	//Parse tag results for next query
	collectionInfo, MaxCount, err := database.DBInterface.GetCollections(pageStart, pageStride)
	if err != nil {
		logging.LogInterface.WriteLog("CollectionQueryRouter", "CollectionsRouter", TemplateInput.UserName, "ERROR", []string{"Error getting collection list", err.Error()})
		TemplateInput.Message += "Error getting collections."
	} else {
		TemplateInput.CollectionInfoList = collectionInfo
		TemplateInput.TotalResults = MaxCount
	}

	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), "/collections")

	replyWithTemplate("collections.html", TemplateInput, responseWriter)
}

package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

//CollectionImageOrderGetRouter serves get requests to /collectionorder
func CollectionImageOrderGetRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	var collectionID uint64
	var err error

	if !TemplateInput.IsLoggedOn() {
		//Redirect to logon
		redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to modify collection members", "LogonRequired")
		return
	}

	//Parse Collection ID
	collectionID, err = strconv.ParseUint(request.FormValue("ID"), 10, 32)
	if err != nil {
		TemplateInput.Message += "Failed to get requested collection. "
		redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderFail")
		return
	}

	//Get collection data
	CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
	if err != nil {
		TemplateInput.Message += "Failed to get collection. SQL Error. "
		redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderFail")
		return
	}

	//Validate Permission to Modify
	if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollectionMembers) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
		TemplateInput.Message += "You do not have edit member permission for collection. "
		redirectWithFlash(responseWriter, request, "/collectionimages?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderFail")
		return
	}

	//Fill in TemplateInput
	TemplateInput.CollectionInfo = CollectionInfo
	//Parse tag results for next query
	imageInfo, MaxCount, err := database.DBInterface.GetCollectionMembers(collectionID, 0, 0)
	if err == nil {
		TemplateInput.ImageInfo = imageInfo
		TemplateInput.TotalResults = MaxCount
	} else {
		logging.WriteLog(logging.LogLevelError, "collectionimagerouter/CollectionImageOrderRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get collection images", strconv.FormatUint(collectionID, 10), err.Error()})
		TemplateInput.Message += "Failed to get collection members."
	}

	replyWithTemplate("collectionreorder.html", TemplateInput, responseWriter, request)
}

//CollectionImageOrderPostRouter serves post requests to /collectionorder
func CollectionImageOrderPostRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	var collectionID uint64
	var err error

	if !TemplateInput.IsLoggedOn() {
		//Redirect to logon
		redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to modify collection members", "LogonRequired")
		return
	}

	//Parse Collection ID
	collectionID, err = strconv.ParseUint(request.FormValue("ID"), 10, 32)
	if err != nil {
		TemplateInput.Message += "Failed to get requested collection. "
		redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderFail")
		return
	}

	//Get collection data
	CollectionInfo, err := database.DBInterface.GetCollection(collectionID)
	if err != nil {
		TemplateInput.Message += "Failed to get collection. SQL Error. "
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to modify collection. "+request.FormValue("ID")+", "+err.Error())
		redirectWithFlash(responseWriter, request, "/collections?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderFail")
		return
	}

	//Validate Permission to Modify
	if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyCollectionMembers) != true && (config.Configuration.UsersControlOwnObjects != true || CollectionInfo.UploaderID != TemplateInput.UserInformation.ID) {
		TemplateInput.Message += "You do not have edit member permission for collection. "
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-COLLECTIONMEMBER", TemplateInput.UserInformation.Name+" failed to modify collection order. Insufficient permissions. "+request.FormValue("ID"))
		redirectWithFlash(responseWriter, request, "/collectionimages?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderFail")
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
		TemplateInput.Message += "Collection re-ordered successfully. "
		redirectWithFlash(responseWriter, request, "/collectionimages?ID="+request.FormValue("ID")+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderSuccess")
		return
	}
	TemplateInput.Message += "No command given in form. "
	redirectWithFlash(responseWriter, request, "/collectionimages?ID="+strconv.FormatUint(collectionID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.Message, "OrderFail")
	return
}

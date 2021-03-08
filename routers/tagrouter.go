package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
)

//TagGetRouter serves get requests to /tag (single tag information)
func TagGetRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)

	requestedID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Error parsing tag id.<br>")
		logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse tag id ", err.Error()})
		redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
		return
	}

	//Populate Tag
	tag, err := database.DBInterface.GetTag(requestedID, true)
	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Error pulling tag.<br>")
		logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to pull tags ", err.Error()})
		redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
		return
	}

	if tag.IsAlias {
		aliasInfo, err := database.DBInterface.GetTag(tag.AliasedID, true)
		TemplateInput.AliasTagInfo = aliasInfo
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Tag is an alias, error pulling parent tag's information.<br>")
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to pull tags alias ", err.Error()})
		}
		TemplateInput.AliasTagInfo = aliasInfo
	}
	TemplateInput.TagContentInfo = tag

	replyWithTemplate("tag.html", TemplateInput, responseWriter, request)
}

//TagPostRouter serves post requests to /tag (single tag information)
func TagPostRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)

	switch cmd := request.FormValue("command"); cmd {
	case "updateTag":
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonRequired")
			return
		}

		requestedID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing tag id.<br>")
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse tag id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		tagInfo, err := database.DBInterface.GetTag(requestedID, false)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error getting tag info.<br>")
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get tag info ", err.Error()})
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}

		//Validate permission to upload
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserInformation.ID != tagInfo.UploaderID) {
			TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for tags.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-TAG", TemplateInput.UserInformation.Name+" failed to update tag. Insufficient permissions. "+strconv.FormatUint(requestedID, 10))
			redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		// /ValidatePermission

		var aliasedTags []interfaces.TagInformation
		var aliasID uint64
		//Get alias information if needed
		if request.FormValue("aliasedTagName") != "" {
			aliasedTags, err = database.DBInterface.GetQueryTags(request.FormValue("aliasedTagName"), false)
			if err != nil || len(aliasedTags) != 1 {
				TemplateInput.HTMLMessage += template.HTML("Error parsing alias information. Ensure you are not putting in multiple tags to alias, and that you are not pointing the alias to an alias.<br>")
				logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse alias id "})
				break
			}
			aliasID = aliasedTags[0].ID
		}
		//Update tag
		if err := database.DBInterface.UpdateTag(requestedID, request.FormValue("tagName"), request.FormValue("tagDescription"), aliasID, len(aliasedTags) == 1, TemplateInput.UserInformation.ID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to update tag. Is your name too short? Did it exist in the first place?<br>")
			redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Tag updated successfully.<br>")
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "MODIFY-TAG", TemplateInput.UserInformation.Name+" successfully updated tag. "+strconv.FormatUint(requestedID, 10)+" to alias "+request.FormValue("aliasedTagName")+" with name "+request.FormValue("tagName")+" and description "+request.FormValue("tagDescription"))
		redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagSucceeded")
		return
	case "bulkAddTag":
		oldTagQuery := request.FormValue("tagName")
		newTagQuery := request.FormValue("newTagName")
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
			break
		}

		if oldTagQuery == "" || newTagQuery == "" {
			TemplateInput.HTMLMessage += template.HTML("You must complete the full form before this action can be performed.<br>")
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		//Validate permission to bulk modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true || TemplateInput.UserPermissions.HasPermission(interfaces.BulkTagOperations) != true {
			TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for bulk tagging on images.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "ADD-BULKIMAGETAG", TemplateInput.UserInformation.Name+" failed to add tag to images. Insufficient permissions. "+oldTagQuery+"->"+newTagQuery)
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		//Parse out tag arguments
		userOldQTags, err := database.DBInterface.GetQueryTags(oldTagQuery, false)
		userNewQTags, err2 := database.DBInterface.GetQueryTags(newTagQuery, false)
		if err != nil || err2 != nil || len(userOldQTags) != 1 || len(userNewQTags) != 1 || userOldQTags[0].Exists == false || userNewQTags[0].Exists == false || userOldQTags[0].ID == userNewQTags[0].ID {
			TemplateInput.HTMLMessage += template.HTML("Failed to get tags from user input. Ensure the tags you entered exist and that you did not enter more than one per field. And that the new and old tags are not the same tag or alias to the same tag.<br>")
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}

		//Confirmed tags exist and are valid
		err = database.DBInterface.BulkAddTag(userNewQTags[0].ID, userOldQTags[0].ID, TemplateInput.UserInformation.ID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error adding tags (SQL).<br>")
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/bulkAddTag", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to bulk add tags due to a SQL error", err.Error(), newTagQuery, oldTagQuery})
			redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(userNewQTags[0].ID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Tags added successfully.<br>")
		go WriteAuditLog(TemplateInput.UserInformation.ID, "ADD-BULKIMAGETAG", TemplateInput.UserInformation.Name+" bulk added tags to images. "+oldTagQuery+"->"+newTagQuery)
		redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(userNewQTags[0].ID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagSucceeded")
		return
	case "replaceTag":
		oldTagQuery := request.FormValue("tagName")
		newTagQuery := request.FormValue("newTagName")
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonRequired")
			return
		}

		if oldTagQuery == "" || newTagQuery == "" {
			TemplateInput.HTMLMessage += template.HTML("You must complete the full form before this action can be performed.<br>")
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		//Validate permission to bulk modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true || TemplateInput.UserPermissions.HasPermission(interfaces.BulkTagOperations) != true {
			TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for bulk tagging on images.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REPLACE-BULKIMAGETAG", TemplateInput.UserInformation.Name+" failed to add tag to images. Insufficient permissions. "+oldTagQuery+"->"+newTagQuery)
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		//Parse out tag arguments
		userOldQTags, err := database.DBInterface.GetQueryTags(oldTagQuery, false)
		userNewQTags, err2 := database.DBInterface.GetQueryTags(newTagQuery, false)
		if err != nil || err2 != nil || len(userOldQTags) != 1 || len(userNewQTags) != 1 || userOldQTags[0].Exists == false || userNewQTags[0].Exists == false || userOldQTags[0].ID == userNewQTags[0].ID {
			TemplateInput.HTMLMessage += template.HTML("Failed to get tags from user input. Ensure the tags you entered exist and that you did not enter more than one per field. And that the new and old tags are not the same tag or alias to the same tag.<br>")
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}

		//Confirmed tags exist and are valid, now replace
		err = database.DBInterface.ReplaceImageTags(userOldQTags[0].ID, userNewQTags[0].ID, TemplateInput.UserInformation.ID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error adding tags (SQL).<br>")
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/replaceTag", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to bulk replace tags due to a SQL error", err.Error(), newTagQuery, oldTagQuery})
			redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(userOldQTags[0].ID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}

		TemplateInput.HTMLMessage += template.HTML("Tags replaced successfully.<br>")
		go WriteAuditLog(TemplateInput.UserInformation.ID, "REPLACE-BULKIMAGETAG", TemplateInput.UserInformation.Name+" bulk added tags to images. "+oldTagQuery+"->"+newTagQuery)
		redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(userNewQTags[0].ID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagSucceeded")
		return
	case "delete":
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonRequired")
			return
		}

		requestedID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing tag id.<br>")
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/delete", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse tag id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		tagInfo, err := database.DBInterface.GetTag(requestedID, false)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error getting tag info.<br>")
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/delete", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get tag info ", err.Error()})
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}

		//Validate permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserInformation.ID != tagInfo.UploaderID) {
			TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for tags.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-TAG", TemplateInput.UserInformation.Name+" failed to delete tag. Insufficient permissions. "+strconv.FormatUint(requestedID, 10))
			redirectWithFlash(responseWriter, request, "/tag?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}
		// /ValidatePermission

		//Update tag
		if err := database.DBInterface.DeleteTag(requestedID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete tag. Ensure the tag is not currently in use.<br>")
			redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
			return
		}

		TemplateInput.HTMLMessage += template.HTML("Tag deleted successfully.<br>")
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-TAG", TemplateInput.UserInformation.Name+" successfully deleted tag. "+strconv.FormatUint(requestedID, 10))
		//redirect user to tags since we just deleted this one
		redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteSuccess")
		return
	}

	TemplateInput.HTMLMessage += template.HTML("Unknown command given.<br>")
	redirectWithFlash(responseWriter, request, "/tags?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "TagFail")
}

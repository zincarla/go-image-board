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

//TagsRouter serves requests to /tags (Big tag list)
func TagsRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	TemplateInput.TotalResults = 0

	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}

	TagSearch := strings.TrimSpace(request.FormValue("SearchTags"))

	//Get the page offset
	pageStartS := request.FormValue("PageStart")
	pageStart, _ := strconv.ParseUint(pageStartS, 10, 32) // Defaults to 0 on error, which is fine
	pageStride := config.Configuration.PageStride

	//Populate Tags
	tag, totalResults, err := database.DBInterface.SearchTags(TagSearch, pageStart, pageStride, false, false)
	if err != nil {
		TemplateInput.Message = "Error pulling tags"
		logging.WriteLog(logging.LogLevelError, "tagrouter/TagsRouter", "", logging.ResultFailure, []string{"Failed to pull tags ", err.Error()})
	} else {
		TemplateInput.Tags = tag
		TemplateInput.TotalResults = totalResults
	}

	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTags="+url.QueryEscape(TagSearch), "/tags")

	replyWithTemplate("tags.html", TemplateInput, responseWriter)
}

//TagRouter serves requests to /tag (single tag information)
func TagRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}
	ID := request.FormValue("ID")

	switch cmd := request.FormValue("command"); cmd {
	case "updateTag":
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action. "
			break
		}
		if ID == "" {
			TemplateInput.Message += "No ID provided to update. "
			break
		}
		iID, err := strconv.ParseUint(ID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id. "
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", "", logging.ResultFailure, []string{"Failed to parse tag id ", err.Error()})
			break
		}
		tagInfo, err := database.DBInterface.GetTag(iID, false)
		if err != nil {
			TemplateInput.Message += "Error getting tag info. "
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", "", logging.ResultFailure, []string{"Failed to get tag info ", err.Error()})
			break
		}

		//Validate permission to upload
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != tagInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags. "
			go WriteAuditLogByName(TemplateInput.UserName, "MODIFY-TAG", TemplateInput.UserName+" failed to update tag. Insufficient permissions. "+ID)
			break
		}
		// /ValidatePermission

		var aliasedTags []interfaces.TagInformation
		var aliasID uint64
		//Get alias information if needed
		if request.FormValue("aliasedTagName") != "" {
			aliasedTags, err = database.DBInterface.GetQueryTags(request.FormValue("aliasedTagName"), false)
			if err != nil || len(aliasedTags) != 1 {
				TemplateInput.Message += "Error parsing alias information. Ensure you are not putting in multiple tags to alias, and that you are not pointing the alias to an alias."
				logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", "", logging.ResultFailure, []string{"Failed to parse alias id "})
				break
			}
			aliasID = aliasedTags[0].ID
		}
		//Update tag
		if err := database.DBInterface.UpdateTag(iID, request.FormValue("tagName"), request.FormValue("tagDescription"), aliasID, len(aliasedTags) == 1, TemplateInput.UserID); err != nil {
			TemplateInput.Message += "Failed to update tag. Is your name too short? Did it exist in the first place? "
		} else {
			TemplateInput.Message += "Tag updated successfully. "
			go WriteAuditLogByName(TemplateInput.UserName, "MODIFY-TAG", TemplateInput.UserName+" successfully updated tag. "+ID+" to alias "+request.FormValue("aliasedTagName")+" with name "+request.FormValue("tagName")+" and description "+request.FormValue("tagDescription"))
		}
	case "bulkAddTag":
		oldTagQuery := request.FormValue("tagName")
		newTagQuery := request.FormValue("newTagName")
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			break
		}
		//Translate UserID
		userID, err := database.DBInterface.GetUserID(TemplateInput.UserName)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/bulkAddTag", TemplateInput.UserName, logging.ResultFailure, []string{"Could not get valid user id", err.Error()})
			TemplateInput.Message += "You muse be logged in to perform that action"
			break
		}
		if oldTagQuery == "" || newTagQuery == "" {
			//redirect to images
			TemplateInput.Message += "You must complete the full form before this action can be performed"
			break
		}
		//Validate permission to bulk modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true || TemplateInput.UserPermissions.HasPermission(interfaces.BulkTagOperations) != true {
			TemplateInput.Message += "User does not have modify permission for bulk tagging on images. "
			go WriteAuditLogByName(TemplateInput.UserName, "ADD-BULKIMAGETAG", TemplateInput.UserName+" failed to add tag to images. Insufficient permissions. "+oldTagQuery+"->"+newTagQuery)
			break
		}
		//Parse out tag arguments
		userOldQTags, err := database.DBInterface.GetQueryTags(oldTagQuery, false)
		userNewQTags, err2 := database.DBInterface.GetQueryTags(newTagQuery, false)
		if err != nil || err2 != nil || len(userOldQTags) != 1 || len(userNewQTags) != 1 || userOldQTags[0].Exists == false || userNewQTags[0].Exists == false || userOldQTags[0].ID == userNewQTags[0].ID {
			TemplateInput.Message += "Failed to get tags from user input. Ensure the tags you entered exist and that you did not enter more than one per field. And that the new and old tags are not the same tag or alias to the same tag. "
			break
		}

		//Confirmed tags exist and are valid
		err = database.DBInterface.BulkAddTag(userNewQTags[0].ID, userOldQTags[0].ID, userID)
		if err != nil {
			TemplateInput.Message += "Error adding tags (SQL). "
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/bulkAddTag", TemplateInput.UserName, logging.ResultFailure, []string{"Failed to bulk add tags due to a SQL error", err.Error(), newTagQuery, oldTagQuery})
		} else {
			TemplateInput.Message += "Tags added successfully. "
			go WriteAuditLog(userID, "ADD-BULKIMAGETAG", TemplateInput.UserName+" bulk added tags to images. "+oldTagQuery+"->"+newTagQuery)
		}
		ID = strconv.FormatUint(userNewQTags[0].ID, 10)
	case "replaceTag":
		oldTagQuery := request.FormValue("tagName")
		newTagQuery := request.FormValue("newTagName")
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			break
		}
		//Translate UserID
		userID, err := database.DBInterface.GetUserID(TemplateInput.UserName)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/replaceTag", TemplateInput.UserName, logging.ResultFailure, []string{"Could not get valid user id", err.Error()})
			TemplateInput.Message += "You muse be logged in to perform that action"
			break
		}
		if oldTagQuery == "" || newTagQuery == "" {
			//redirect to images
			TemplateInput.Message += "You must complete the full form before this action can be performed"
			break
		}
		//Validate permission to bulk modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true || TemplateInput.UserPermissions.HasPermission(interfaces.BulkTagOperations) != true {
			TemplateInput.Message += "User does not have modify permission for bulk tagging on images. "
			go WriteAuditLogByName(TemplateInput.UserName, "REPLACE-BULKIMAGETAG", TemplateInput.UserName+" failed to add tag to images. Insufficient permissions. "+oldTagQuery+"->"+newTagQuery)
			break
		}
		//Parse out tag arguments
		userOldQTags, err := database.DBInterface.GetQueryTags(oldTagQuery, false)
		userNewQTags, err2 := database.DBInterface.GetQueryTags(newTagQuery, false)
		if err != nil || err2 != nil || len(userOldQTags) != 1 || len(userNewQTags) != 1 || userOldQTags[0].Exists == false || userNewQTags[0].Exists == false || userOldQTags[0].ID == userNewQTags[0].ID {
			TemplateInput.Message += "Failed to get tags from user input. Ensure the tags you entered exist and that you did not enter more than one per field. And that the new and old tags are not the same tag or alias to the same tag. "
			break
		}

		//Confirmed tags exist and are valid
		err = database.DBInterface.ReplaceImageTags(userOldQTags[0].ID, userNewQTags[0].ID, userID)
		if err != nil {
			TemplateInput.Message += "Error adding tags (SQL). "
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/replaceTag", TemplateInput.UserName, logging.ResultFailure, []string{"Failed to bulk replace tags due to a SQL error", err.Error(), newTagQuery, oldTagQuery})
		} else {
			TemplateInput.Message += "Tags replaced successfully. "
			go WriteAuditLog(userID, "REPLACE-BULKIMAGETAG", TemplateInput.UserName+" bulk added tags to images. "+oldTagQuery+"->"+newTagQuery)
		}
		ID = strconv.FormatUint(userNewQTags[0].ID, 10)
	case "delete":
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action. "
			break
		}
		if ID == "" {
			TemplateInput.Message += "No ID provided to delete. "
			break
		}
		iID, err := strconv.ParseUint(ID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id. "
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/delete", "", logging.ResultFailure, []string{"Failed to parse tag id ", err.Error()})
			break
		}
		tagInfo, err := database.DBInterface.GetTag(iID, false)
		if err != nil {
			TemplateInput.Message += "Error getting tag info. "
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter/delete", "", logging.ResultFailure, []string{"Failed to get tag info ", err.Error()})
			break
		}

		//Validate permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != tagInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags. "
			go WriteAuditLogByName(TemplateInput.UserName, "DELETE-TAG", TemplateInput.UserName+" failed to delete tag. Insufficient permissions. "+ID)
			break
		}
		// /ValidatePermission

		//Update tag
		if err := database.DBInterface.DeleteTag(iID); err != nil {
			TemplateInput.Message += "Failed to delete tag. Ensure the tag is not currently in use. "
		} else {
			TemplateInput.Message += "Tag deleted successfully. "
			go WriteAuditLogByName(TemplateInput.UserName, "DELETE-TAG", TemplateInput.UserName+" successfully deleted tag. "+ID)
			//redirect user to tags since we just deleted this one
			http.Redirect(responseWriter, request, "/tags?&prevMessage="+url.QueryEscape(TemplateInput.Message)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), 302)
			return
		}

	}

	if ID == "" {
		TemplateInput.Message += "Error parsing tag id. "
	} else {
		iID, err := strconv.ParseUint(ID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id. "
			logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", "", logging.ResultFailure, []string{"Failed to parse tag id ", err.Error()})
		} else {
			//Populate Tag
			tag, err := database.DBInterface.GetTag(iID, true)
			if err != nil {
				TemplateInput.Message += "Error pulling tag"
				logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", "", logging.ResultFailure, []string{"Failed to pull tags ", err.Error()})
			} else {
				if tag.IsAlias {
					aliasInfo, err := database.DBInterface.GetTag(tag.AliasedID, true)
					TemplateInput.AliasTagInfo = aliasInfo
					if err != nil {
						TemplateInput.Message += "Error pulling tag alias information. "
						logging.WriteLog(logging.LogLevelError, "tagrouter/TagRouter", "", logging.ResultFailure, []string{"Failed to pull tags alias ", err.Error()})
					} else {
						TemplateInput.TagContentInfo = tag
						TemplateInput.AliasTagInfo = aliasInfo
					}
				} else {
					TemplateInput.TagContentInfo = tag
				}
			}
		}
	}
	replyWithTemplate("tag.html", TemplateInput, responseWriter)
}

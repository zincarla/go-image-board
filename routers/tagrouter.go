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

	TagSearch := strings.TrimSpace(request.FormValue("SearchTags"))

	//Get the page offset
	pageStartS := request.FormValue("PageStart")
	pageStart, _ := strconv.ParseUint(pageStartS, 10, 32) // Defaults to 0 on error, which is fine
	pageStride := config.Configuration.PageStride

	//Populate Tags
	tag, TemplateInput.TotalResults, err := database.DBInterface.SearchTags(TagSearch, pageStart, pageStride)
	if err != nil {
		TemplateInput.Message = "Error pulling tags"
		logging.LogInterface.WriteLog("TagsRouter", "TagsRouter", "*", "ERROR", []string{"Failed to pull tags ", err.Error()})
	} else {
		TemplateInput.Tags = tag
	}

	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTags="+url.QueryEscape(TagSearch), "/tags")

	replyWithTemplate("tags.html", TemplateInput, responseWriter)
}

//TagRouter serves requests to /tag (single tag information)
func TagRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
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
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse tag id ", err.Error()})
			break
		}
		tagInfo, err := database.DBInterface.GetTag(iID)
		if err != nil {
			TemplateInput.Message += "Error getting tag info. "
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to get tag info ", err.Error()})
			break
		}

		//Validate permission to upload
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != tagInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags. "
			go writeAuditLogByName(TemplateInput.UserName, "MODIFY-TAG", TemplateInput.UserName+" failed to update tag. Insufficient permissions. "+ID)
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
				logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse alias id "})
				break
			}
			aliasID = aliasedTags[0].ID
		}
		//Update tag
		if err := database.DBInterface.UpdateTag(iID, request.FormValue("tagName"), request.FormValue("tagDescription"), aliasID, len(aliasedTags) == 1, TemplateInput.UserID); err != nil {
			TemplateInput.Message += "Failed to update tag. Is your name too short? Did it exist in the first place? "
		} else {
			TemplateInput.Message += "Tag updated successfully. "
			go writeAuditLogByName(TemplateInput.UserName, "MODIFY-TAG", TemplateInput.UserName+" successfully updated tag. "+ID+" to alias "+request.FormValue("aliasedTagName")+" with name "+request.FormValue("tagName")+" and description "+request.FormValue("tagDescription"))
		}
	case "remove":
		ImageID := request.FormValue("ImageID")
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			break
		}
		if ID == "" || ImageID == "" {
			TemplateInput.Message += "No ID provided to remove."
			break
		}

		iImageID, err := strconv.ParseUint(ImageID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id or image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse tag or image id ", err.Error()})
			break
		}

		imageInfo, err := database.DBInterface.GetImage(iImageID)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}

		//Validate permission to upload
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != imageInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags on images. "
			go writeAuditLogByName(TemplateInput.UserName, "REMOVE-IMAGETAG", TemplateInput.UserName+" failed to remove tag from image "+ImageID+". Insufficient permissions. "+ID)
			break
		}
		// /ValidatePermission
		iID, err := strconv.ParseUint(ID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id or image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse tag or image id ", err.Error()})
			break
		}
		//Remove tag
		if err := database.DBInterface.RemoveTag(iID, iImageID); err != nil {
			TemplateInput.Message += "Failed to remove tag. Was it attached in the first place?"
		} else {
			TemplateInput.Message += "Tag removed successfully"
			go writeAuditLogByName(TemplateInput.UserName, "REMOVE-IMAGETAG", TemplateInput.UserName+" removed tag from image "+ImageID+". tag "+ID)
		}
		//Redirect back to image
		http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		return
	case "AddTags":
		ImageID := request.FormValue("ImageID")
		userQuery := request.FormValue("NewTags")
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			break
		}
		//Translate UserID
		userID, err := database.DBInterface.GetUserID(TemplateInput.UserName)
		if err != nil {
			logging.LogInterface.WriteLog("TagRouter", "AddTags", TemplateInput.UserName, "ERROR", []string{"Could not get valid user id", err.Error()})
			TemplateInput.Message += "You muse be logged in to perform that action"
			break
		}
		if ImageID == "" {
			//redirect to images
			TemplateInput.Message += "Error parsing image id"
			break
		}

		iImageID, err := strconv.ParseUint(ImageID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}
		imageInfo, err := database.DBInterface.GetImage(iImageID)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			break
		}
		//Validate permission to modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != imageInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags on images. "
			go writeAuditLogByName(TemplateInput.UserName, "ADD-IMAGETAG", TemplateInput.UserName+" failed to add tag to image "+ImageID+". Insufficient permissions. "+userQuery)

			break
		}
		// /ValidatePermission
		//Create tags
		//Get tags
		userQTags, err := database.DBInterface.GetQueryTags(userQuery, false)
		if err != nil {
			TemplateInput.Message += "Failed to get tags from input"
			break
		}
		for _, tag := range userQTags {
			if tag.Exists {
				if tag.IsAlias == false {
					//Assign pre-existing tag //Permission note, we already validated user has ModifyImageTags
					if err := database.DBInterface.AddTag(tag.ID, iImageID, userID); err != nil {
						logging.LogInterface.WriteLog("TagRouter", "AddTags", TemplateInput.UserName, "WARNING", []string{"error attempting to add tag to new file", err.Error(), strconv.FormatUint(iImageID, 10), strconv.FormatUint(tag.ID, 10)})
						TemplateInput.Message += err.Error()
					} else {
						go writeAuditLogByName(TemplateInput.UserName, "ADD-IMAGETAG", TemplateInput.UserName+" successfully added tag to image "+ImageID+". tag "+tag.Name)
						TemplateInput.Message += "Successfully added " + tag.Name + ". "
					}
				}
			} else {
				//Create Tag //Permission note, we need to validate if user can add new tags
				if TemplateInput.UserPermissions.HasPermission(interfaces.AddTags) != true {
					TemplateInput.Message += "User does not have create permission for tags. (" + tag.Name + ") "
				} else {
					tagID, err := database.DBInterface.NewTag(tag.Name, tag.Description, userID)
					if err != nil {
						TemplateInput.Message += err.Error()
						logging.LogInterface.WriteLog("TagRouter", "AddTags", TemplateInput.UserName, "WARNING", []string{"error attempting to create tag for new file", err.Error(), strconv.FormatUint(iImageID, 10), tag.Name})
					} else {
						if err := database.DBInterface.AddTag(tagID, iImageID, userID); err != nil {
							TemplateInput.Message += err.Error()
							logging.LogInterface.WriteLog("TagRouter", "AddTags", TemplateInput.UserName, "WARNING", []string{"error attempting to add newly created tag to new file", err.Error(), strconv.FormatUint(iImageID, 10), strconv.FormatUint(tagID, 10)})
						} else {
							TemplateInput.Message += "Successfully added " + tag.Name + ". "
						}
					}
				}
			}
		}
		//Link tags
		//Redirect back to image
		http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		return
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
			logging.LogInterface.WriteLog("TagRouter", "bulkAddTag", TemplateInput.UserName, "ERROR", []string{"Could not get valid user id", err.Error()})
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
			go writeAuditLogByName(TemplateInput.UserName, "ADD-BULKIMAGETAG", TemplateInput.UserName+" failed to add tag to images. Insufficient permissions. "+oldTagQuery+"->"+newTagQuery)
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
			logging.LogInterface.WriteLog("tagrouter", "TagRouter", TemplateInput.UserName, "ERROR", []string{"Failed to bulk add tags due to a SQL error", err.Error(), newTagQuery, oldTagQuery})
		} else {
			TemplateInput.Message += "Tags added successfully. "
			go writeAuditLog(userID, "ADD-BULKIMAGETAG", TemplateInput.UserName+" bulk added tags to images. "+oldTagQuery+"->"+newTagQuery)
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
			logging.LogInterface.WriteLog("TagRouter", "replaceTag", TemplateInput.UserName, "ERROR", []string{"Could not get valid user id", err.Error()})
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
			go writeAuditLogByName(TemplateInput.UserName, "REPLACE-BULKIMAGETAG", TemplateInput.UserName+" failed to add tag to images. Insufficient permissions. "+oldTagQuery+"->"+newTagQuery)
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
			logging.LogInterface.WriteLog("tagrouter", "TagRouter", TemplateInput.UserName, "ERROR", []string{"Failed to bulk replace tags due to a SQL error", err.Error(), newTagQuery, oldTagQuery})
		} else {
			TemplateInput.Message += "Tags replaced successfully. "
			go writeAuditLog(userID, "REPLACE-BULKIMAGETAG", TemplateInput.UserName+" bulk added tags to images. "+oldTagQuery+"->"+newTagQuery)
		}
		ID = strconv.FormatUint(userNewQTags[0].ID, 10)
	case "ChangeRating":
		ImageID := request.FormValue("ImageID")
		newRating := strings.ToLower(request.FormValue("NewRating"))
		if TemplateInput.UserName == "" {
			TemplateInput.Message += "You must be logged in to perform that action"
			http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
			return
		}

		if ImageID == "" {
			//redirect to images
			TemplateInput.Message += "Error parsing image id"
			http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
			return
		}

		iImageID, err := strconv.ParseUint(ImageID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
			return
		}
		imageInfo, err := database.DBInterface.GetImage(iImageID)
		if err != nil {
			TemplateInput.Message += "Error parsing image id"
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse image id ", err.Error()})
			http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
			return
		}
		//Validate permission to modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserID != imageInfo.UploaderID) {
			TemplateInput.Message += "User does not have modify permission for tags on images. "
			go writeAuditLogByName(TemplateInput.UserName, "ADD-IMAGERATING", TemplateInput.UserName+" failed to edit rating for image "+ImageID+". Insufficient permissions. "+newRating)
			http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
			return
		}
		// /ValidatePermission
		//Change Rating

		if err = database.DBInterface.SetImageRating(iImageID, newRating); err != nil {
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to change image rating ", err.Error()})
			TemplateInput.Message += "Failed to change image rating, internal error ocurred. "
		}

		//Redirect back to image
		http.Redirect(responseWriter, request, "/image?ID="+ImageID+"&prevMessage="+url.QueryEscape(TemplateInput.Message), 302)
		return
	}

	if ID == "" {
		TemplateInput.Message += "Error parsing tag id. "
	} else {
		iID, err := strconv.ParseUint(ID, 10, 32)
		if err != nil {
			TemplateInput.Message += "Error parsing tag id. "
			logging.LogInterface.WriteLog("TagsRouter", "TagRouter", "*", "ERROR", []string{"Failed to parse tag id ", err.Error()})
		} else {
			//Populate Tag
			tag, err := database.DBInterface.GetTag(iID)
			if err != nil {
				TemplateInput.Message += "Error pulling tag"
				logging.LogInterface.WriteLog("TagsRouter", "TagsRouter", "*", "ERROR", []string{"Failed to pull tags ", err.Error()})
			} else {
				if tag.IsAlias {
					aliasInfo, err := database.DBInterface.GetTag(tag.AliasedID)
					TemplateInput.AliasTagInfo = aliasInfo
					if err != nil {
						TemplateInput.Message += "Error pulling tag alias information. "
						logging.LogInterface.WriteLog("TagsRouter", "TagsRouter", "*", "ERROR", []string{"Failed to pull tags alias ", err.Error()})
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

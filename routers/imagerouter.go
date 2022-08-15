package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers/templatecache"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

//ImageGetRouter serves get requests to /image
func ImageGetRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	var requestedID uint64
	var err error

	//Change StreamView if requested
	if request.FormValue("ViewMode") == "stream" {
		TemplateInput.ViewMode = "stream"
		_, _, session := getSessionInformation(request)
		session.Values["ViewMode"] = "stream"
		session.Save(request, responseWriter)
	} else if request.FormValue("ViewMode") == "slideshow" {
		TemplateInput.ViewMode = "slideshow"
		_, _, session := getSessionInformation(request)
		session.Values["ViewMode"] = "slideshow"
		if request.FormValue("slideshowspeed") != "" {
			parsedSSS, err := strconv.ParseInt(request.FormValue("slideshowspeed"), 10, 64)
			if err == nil && parsedSSS > 0 {
				session.Values["slideshowspeed"] = parsedSSS
			}
		}
		session.Save(request, responseWriter)
	} else if request.FormValue("ViewMode") != "" { //default to grid on invalid modes
		TemplateInput.ViewMode = "grid"
		_, _, session := getSessionInformation(request)
		session.Values["ViewMode"] = "grid"
		session.Save(request, responseWriter)
	}

	//ID should come from request
	requestedID, err = strconv.ParseUint(request.FormValue("ID"), 10, 32)
	if err != nil {
		//No ID? Redirect to image search
		TemplateInput.HTMLMessage += template.HTML("No image selected or image not found.<br>")
		redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "ImageFail")
		return
	}

	//Get Imageinformation
	imageInfo, err := database.DBInterface.GetImage(requestedID)
	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Failed to get image information.<br>")
		logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get image info for", strconv.FormatUint(requestedID, 10), err.Error()})
		redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "ImageFail")
		return
	}

	//Get Collection Info
	imageInfo.MemberCollections, err = database.DBInterface.GetCollectionsWithImage(requestedID)
	if err != nil {
		//log err but no need to inform user
		logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get collection info for", strconv.FormatUint(requestedID, 10), err.Error()})
	}

	//Get next and previous image based on query
	userQTags := []interfaces.TagInformation{}
	err = nil
	if TemplateInput.OldQuery != "" {
		userQTags, err = database.DBInterface.GetQueryTags(TemplateInput.OldQuery, false)
	}
	if err == nil {
		//if signed in, add user's global filters to query
		if TemplateInput.UserInformation.Name != "" {
			userFilterTags, err := database.DBInterface.GetUserFilterTags(TemplateInput.UserInformation.ID, false)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
				TemplateInput.HTMLMessage += template.HTML("Failed to add your global filter to this query. Internal error.<br>")
			} else {
				userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
			}
		}
		prevNextImage, err := database.DBInterface.GetPrevNexImages(userQTags, requestedID)
		if err == nil {
			if len(prevNextImage) == 2 {
				TemplateInput.NextMemberID = prevNextImage[1].ID
				TemplateInput.PreviousMemberID = prevNextImage[0].ID
			} else if len(prevNextImage) == 1 {
				if prevNextImage[0].ID > requestedID {
					TemplateInput.PreviousMemberID = prevNextImage[0].ID
				} else {
					TemplateInput.NextMemberID = prevNextImage[0].ID
				}
			}
		} else {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to get next/prev image", err.Error()})
		}
	}

	//Set Template with imageInfo
	TemplateInput.ImageContentInfo = imageInfo

	if config.Configuration.ShowSimilarOnImages {
		similarTag, err := database.DBInterface.GetQueryTags("similar:"+strconv.FormatUint(imageInfo.ID, 10), false)
		if err == nil {
			_, similarCount, _ := database.DBInterface.SearchImages(similarTag, 0, config.Configuration.PageStride)
			if similarCount > 1 {
				TemplateInput.SimilarCount = similarCount - 1 //Remove the current image from count
			}
		}
	}

	//Check if source is a url
	if _, err := url.ParseRequestURI(TemplateInput.ImageContentInfo.Source); err == nil {
		//Source is url
		TemplateInput.ImageContentInfo.SourceIsURL = true
	}

	//Get vote information if logged in
	if TemplateInput.IsLoggedOn() {
		TemplateInput.ImageContentInfo.UsersVotedScore, err = database.DBInterface.GetUserVoteScore(TemplateInput.UserInformation.ID, requestedID)
	}

	//Get the image content information based on type (Img, vs video vs...)
	TemplateInput.ImageContent = templatecache.GetEmbedForContent(imageInfo.Location)

	TemplateInput.Tags, err = database.DBInterface.GetImageTags(imageInfo.ID)
	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Failed to load tags.<br>")
		logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to load tags", err.Error()})
	}

	if TemplateInput.ViewMode == "slideshow" {
		replyWithTemplate("image-slideshow-js.html", TemplateInput, responseWriter, request)
		return
	}

	replyWithTemplate("image.html", TemplateInput, responseWriter, request)
}

//ImagePostRouter serves post requests to /image
func ImagePostRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	var requestedID uint64
	var err error
	var duplicateIDs map[string]uint64
	//If we are just now uploading the file, then we need to get ID from upload function
	switch request.FormValue("command") {
	case "uploadFile":
		if TemplateInput.UserInformation.Name == "" {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to upload an image", "LogonRequired")
			return
		}
		logging.WriteLog(logging.LogLevelVerbose, "imagerouter/ImageRouter/uploadFile", TemplateInput.UserInformation.GetCompositeID(), logging.ResultInfo, []string{"Attempting to upload file"})
		requestedID, duplicateIDs, err = handleImageUpload(request, TemplateInput.UserInformation.Name)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/uploadFile", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{err.Error()})
			TemplateInput.HTMLMessage += template.HTML("One or more warnings generated during upload: " + html.EscapeString(err.Error()))
		}
		if duplicateIDs != nil && len(duplicateIDs) > 0 {
			for fileName, duplicateID := range duplicateIDs {
				TemplateInput.HTMLMessage += template.HTML("<a href=\"/image?ID=" + strconv.FormatUint(duplicateID, 10) + "\">" + template.HTMLEscapeString(fileName) + "</a> has already been uploaded. ")
			}
		}
		//Nicety for if we have blank requestID
		if requestedID == 0 && duplicateIDs != nil && len(duplicateIDs) > 0 {
			for _, duplicateID := range duplicateIDs {
				requestedID = duplicateID
				break
			}
		}
		TemplateInput.HTMLMessage += template.HTML("Upload complete. ")
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UploadFinished")
		return
	case "ChangeVote":
		sImageID := request.FormValue("ID")
		if TemplateInput.UserInformation.Name == "" || TemplateInput.UserInformation.ID == 0 {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to vote on an image", "LogonRequired")
			return
		}
		logging.WriteLog(logging.LogLevelVerbose, "imagerouter/ImageRouter/ChangeVote", TemplateInput.UserInformation.GetCompositeID(), logging.ResultInfo, []string{"Attempting to vote on image"})

		requestedID, err = strconv.ParseUint(sImageID, 10, 64)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse imageid to vote on"})
			TemplateInput.HTMLMessage += template.HTML("Failed to parse image id to vote on.<br>")
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		//Validate permission to vote
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get image information.<br>")
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}

		if !(TemplateInput.UserPermissions.HasPermission(interfaces.ScoreImage) || (imageInfo.UploaderID == TemplateInput.UserInformation.ID && config.Configuration.UsersControlOwnObjects)) {
			go WriteAuditLog(TemplateInput.UserInformation.ID, "IMAGE-SCORE", TemplateInput.UserInformation.Name+" failed to score image. No permissions.")
			TemplateInput.HTMLMessage += template.HTML("You do not have permissions to vote on this image.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		// /ValidatePermission

		//At this point, user is validated
		Score, err := strconv.ParseInt(request.FormValue("NewVote"), 10, 64)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to parse your vote value.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		if Score < -10 || Score > 10 {
			TemplateInput.HTMLMessage += template.HTML("Score must be between -10 and 10.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		if err := database.DBInterface.UpdateUserVoteScore(TemplateInput.UserInformation.ID, requestedID, Score); err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeVota", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to set vote in database", err.Error()})
			TemplateInput.HTMLMessage += template.HTML("Failed to set vote in database, internal error.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Successfully changed vote!<br>")
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateSucceeded")
		return
	case "ChangeSource":
		sImageID := request.FormValue("ID")
		if TemplateInput.UserInformation.Name == "" || TemplateInput.UserInformation.ID == 0 {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to modify an image", "LogonRequired")
			return
		}
		logging.WriteLog(logging.LogLevelVerbose, "imagerouter/ImageRouter/ChangeSource", TemplateInput.UserInformation.GetCompositeID(), logging.ResultInfo, []string{"Attempting to source an image"})

		requestedID, err = strconv.ParseUint(sImageID, 10, 64)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeSource", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse imageid to vote on"})
			TemplateInput.HTMLMessage += template.HTML("Failed to parse image id to vote on.<br>")
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		//Validate permission to vote
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get image information.<br>")
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}

		if !(TemplateInput.UserPermissions.HasPermission(interfaces.SourceImage) || (imageInfo.UploaderID == TemplateInput.UserInformation.ID && config.Configuration.UsersControlOwnObjects)) {
			go WriteAuditLog(TemplateInput.UserInformation.ID, "IMAGE-SOURCE", TemplateInput.UserInformation.Name+" failed to source image. No permissions.")
			TemplateInput.HTMLMessage += template.HTML("You do not have permissions to change the source of this image.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		// /ValidatePermission

		//At this point, user is validated
		Source := request.FormValue("NewSource")

		if err := database.DBInterface.SetImageSource(requestedID, Source); err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeSource", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to set source in database", err.Error()})
			TemplateInput.HTMLMessage += template.HTML("Failed to set source in database, internal error.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Successfully changed source!<br>")
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateSucceeded")
		return
	case "ChangeName":
		sImageID := request.FormValue("ID")
		if TemplateInput.UserInformation.Name == "" || TemplateInput.UserInformation.ID == 0 {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to modify an image", "LogonRequired")
			return
		}
		logging.WriteLog(logging.LogLevelVerbose, "imagerouter/ImageRouter/ChangeName", TemplateInput.UserInformation.GetCompositeID(), logging.ResultInfo, []string{"Attempting to name an image"})

		requestedID, err = strconv.ParseUint(sImageID, 10, 64)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeName", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse imageid to change name on"})
			TemplateInput.HTMLMessage += template.HTML("Failed to parse image id.<br>")
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		//Validate permission to vote
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get image information.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}

		if !(TemplateInput.UserPermissions.HasPermission(interfaces.SourceImage) || (imageInfo.UploaderID == TemplateInput.UserInformation.ID && config.Configuration.UsersControlOwnObjects)) {
			go WriteAuditLog(TemplateInput.UserInformation.ID, "IMAGE-NAME", TemplateInput.UserInformation.Name+" failed to name image. No permissions.")
			TemplateInput.HTMLMessage += template.HTML("You do not have permissions to change the name/description of this image.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		// /ValidatePermission

		//At this point, user is validated
		Name := request.FormValue("NewName")
		Description := request.FormValue("NewDescription")

		if err := database.DBInterface.UpdateImage(requestedID, Name, Description, nil, nil, nil, nil); err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeName", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to set name in database", err.Error()})
			TemplateInput.HTMLMessage += template.HTML("Failed to set name/description in database, internal error.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Successfully changed name/description!<br>")
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateSucceeded")
		return
	case "RemoveTag":
		TagID := request.FormValue("TagID")
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonRequired")
			return
		}

		requestedID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing tag id or image id.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/RemoveTag", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse tag or image id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}

		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing image id.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/RemoveTag", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse image id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}

		//Validate permission to manage tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserInformation.ID != imageInfo.UploaderID) {
			TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for tags on images.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-IMAGETAG", TemplateInput.UserInformation.Name+" failed to remove tag from image "+strconv.FormatUint(requestedID, 10)+". Insufficient permissions. "+TagID)
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		// /ValidatePermission
		requestedTagID, err := strconv.ParseUint(TagID, 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing tag id or image id.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/RemoveTag", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse tag or image id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		//Remove tag
		if err := database.DBInterface.RemoveTag(requestedTagID, requestedID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to remove tag. Was it attached in the first place?<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Tag removed successfully.<br>")
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "REMOVE-IMAGETAG", TemplateInput.UserInformation.Name+" removed tag from image "+strconv.FormatUint(requestedID, 10)+". tag "+TagID)
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateSucceeded")
		return
	case "AddTags":
		userQuery := request.FormValue("NewTags")
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonRequired")
			return
		}

		requestedID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing image id.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/AddTags", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse image id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing image id.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/AddTags", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse image id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		//Validate permission to modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserInformation.ID != imageInfo.UploaderID) {
			TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for tags on images.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "ADD-IMAGETAG", TemplateInput.UserInformation.Name+" failed to add tag to image "+strconv.FormatUint(requestedID, 10)+". Insufficient permissions. "+userQuery)
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		// /ValidatePermission

		///////////////////
		//Get tags
		var validatedUserTags []uint64 //Will contain tags the user is allowed to use
		tagIDString := ""
		userQTags, err := database.DBInterface.GetQueryTags(request.FormValue("NewTags"), false)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get tags from input.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		for _, tag := range userQTags {
			if tag.Exists && tag.IsMeta == false {
				//Assign pre-existing tag
				//Permissions to tag validated above
				validatedUserTags = append(validatedUserTags, tag.ID)
				tagIDString = tagIDString + ", " + strconv.FormatUint(tag.ID, 10)
			} else if tag.IsMeta == false {
				//Create Tag
				//Validate permissions to create tags
				if TemplateInput.UserPermissions.HasPermission(interfaces.AddTags) != true {
					logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/AddTags", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Does not have create tag permission"})
					TemplateInput.HTMLMessage += template.HTML("Unable to use tag " + template.HTMLEscapeString(tag.Name) + " due to insufficient permissions of user to create tags.<br>")
					// /ValidatePermission
				} else {
					tagID, err := database.DBInterface.NewTag(tag.Name, tag.Description, TemplateInput.UserInformation.ID)
					if err != nil {
						logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/AddTags", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"error attempting to create tag", err.Error(), tag.Name})
						TemplateInput.HTMLMessage += template.HTML("Unable to use tag " + template.HTMLEscapeString(tag.Name) + " due to a database error.<br>")
					} else {
						go WriteAuditLog(TemplateInput.UserInformation.ID, "CREATE-TAG", TemplateInput.UserInformation.Name+" created a new tag. "+tag.Name)
						validatedUserTags = append(validatedUserTags, tagID)
						tagIDString = tagIDString + ", " + strconv.FormatUint(tagID, 10)
					}
				}
			}
		}
		///////////////////
		if err := database.DBInterface.AddTag(validatedUserTags, requestedID, TemplateInput.UserInformation.ID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to add tag due to database error.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/AddTags", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"error attempting to add tags to file", err.Error(), strconv.FormatUint(requestedID, 10), tagIDString})
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Tag(s) added.<br>")
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateSucceeded")
		return
	case "ChangeRating":
		newRating := strings.ToLower(request.FormValue("NewRating"))
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonRequired")
			return
		}

		requestedID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing image id.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeRating", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse image id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		imageInfo, err := database.DBInterface.GetImage(requestedID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Error parsing image id.<br>")
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeRating", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to parse image id ", err.Error()})
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		//Validate permission to modify tags
		if TemplateInput.UserPermissions.HasPermission(interfaces.ModifyImageTags) != true && (config.Configuration.UsersControlOwnObjects != true || TemplateInput.UserInformation.ID != imageInfo.UploaderID) {
			TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for tags on images.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "ADD-IMAGERATING", TemplateInput.UserInformation.Name+" failed to edit rating for image "+strconv.FormatUint(requestedID, 10)+". Insufficient permissions. "+newRating)
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}

		// /ValidatePermission
		//Change Rating
		if err = database.DBInterface.SetImageRating(requestedID, newRating); err != nil {
			logging.WriteLog(logging.LogLevelError, "imagerouter/ImageRouter/ChangeRating", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to change image rating ", err.Error()})
			TemplateInput.HTMLMessage += template.HTML("Failed to change image rating, internal error ocurred.<br>")
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateFailed")
			return
		}
		TemplateInput.HTMLMessage += template.HTML("Updated rating.<br>")
		redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(requestedID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "UpdateSucceeded")
		return
	case "delete":
		if !TemplateInput.IsLoggedOn() {
			//Redirect to logon
			redirectWithFlash(responseWriter, request, "/logon", "You must be logged in to delete an image", "LogonRequired")
			return
		}
		//Get Image ID
		parsedImageID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to get image with that ID.<br>")
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}
		//Cache image data
		ImageInfo, err := database.DBInterface.GetImage(parsedImageID)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete image. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" failed to delete image. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveImage) != true && (config.Configuration.UsersControlOwnObjects != true || ImageInfo.UploaderID != TemplateInput.UserInformation.ID) {
			TemplateInput.HTMLMessage += template.HTML("You do not have delete permission for this image.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" failed to delete image. Insufficient permissions. "+request.FormValue("ID"))
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}

		//Permission validated, now delete (ImageTags and Images)
		if err := database.DBInterface.DeleteImage(parsedImageID); err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to delete image. SQL Error.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" failed to delete image. "+request.FormValue("ID")+", "+err.Error())
			redirectWithFlash(responseWriter, request, "/image?ID="+strconv.FormatUint(parsedImageID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteFailed")
			return
		}
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "DELETE-IMAGE", TemplateInput.UserInformation.Name+" deleted image. "+request.FormValue("ID")+", "+ImageInfo.Name+", "+ImageInfo.Location)
		//Third, delete Image from Disk
		go os.Remove(path.Join(config.Configuration.ImageDirectory, ImageInfo.Location))
		//Last delete thumbnail from disk
		go os.Remove(path.Join(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+ImageInfo.Location+".png"))
		TemplateInput.HTMLMessage += template.HTML("Deletion success.<br>")
		redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "DeleteSuccess")
		return
	}
	TemplateInput.HTMLMessage += template.HTML("Command not recognized or form submitted incorrectly.<br>")
	redirectWithFlash(responseWriter, request, "/images?SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), TemplateInput.HTMLMessage, "ImageFail")
	return
}

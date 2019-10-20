package routers

import (
	"errors"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

//ImageQueryRouter serves requests to /images
func ImageQueryRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}
	userQuery := TemplateInput.OldQuery

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
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("You must be logged in to delete images"), 302)
			return
		}
		//Get Image ID
		parsedImageID, err := strconv.ParseUint(request.FormValue("ID"), 10, 32)
		if err != nil {
			TemplateInput.Message += "Failed to get image with that ID."
			break
		}
		//Cache image data
		ImageInfo, err := database.DBInterface.GetImage(parsedImageID)
		if err != nil {
			TemplateInput.Message += "Failed to delete image. SQL Error. "
			go writeAuditLogByName(TemplateInput.UserName, "DELETE-IMAGE", TemplateInput.UserName+" failed to delete image. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}

		//Validate Permission to delete
		if TemplateInput.UserPermissions.HasPermission(interfaces.RemoveImage) != true && (config.Configuration.UsersControlOwnObjects != true || ImageInfo.UploaderID != TemplateInput.UserID) {
			TemplateInput.Message += "User does not have delete permission for image. "
			go writeAuditLogByName(TemplateInput.UserName, "DELETE-IMAGE", TemplateInput.UserName+" failed to delete image. Insufficient permissions. "+request.FormValue("ID"))
			break
		}

		//Permission validated, now delete (ImageTags and Images)
		if err := database.DBInterface.DeleteImage(parsedImageID); err != nil {
			TemplateInput.Message += "Failed to delete image. SQL Error. "
			go writeAuditLogByName(TemplateInput.UserName, "DELETE-IMAGE", TemplateInput.UserName+" failed to delete image. "+request.FormValue("ID")+", "+err.Error())
			break //Cancel delete
		}
		go writeAuditLogByName(TemplateInput.UserName, "DELETE-IMAGE", TemplateInput.UserName+" deleted image. "+request.FormValue("ID")+", "+ImageInfo.Name+", "+ImageInfo.Location)
		//Third, delete Image from Disk
		go os.Remove(config.JoinPath(config.Configuration.ImageDirectory, ImageInfo.Location))
		//Last delete thumbnail from disk
		go os.Remove(config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+ImageInfo.Location+".png"))
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
	//logging.LogInterface.WriteLog("ImageRouter", "ImageQueryRouter", "*", "INFO", []string{"User attempting a query", userQuery})
	//Cleanup and format tags for use with SearchImages
	userQTags, err := database.DBInterface.GetQueryTags(userQuery, false)
	if err == nil {
		//if signed in, add user's global filters to query
		if TemplateInput.UserName != "" {
			userFilterTags, err := database.DBInterface.GetUserFilterTags(TemplateInput.UserID, false)
			if err != nil {
				logging.LogInterface.WriteLog("ImageRouter", "ImageQueryRouter", TemplateInput.UserName, "ERROR", []string{"Failed to load user's filter", err.Error()})
				TemplateInput.Message += "Failed to add your global filter to this query. Internal error. "
			} else {
				userQTags = append(userQTags, userFilterTags...)
			}
		}
		//Return random image if requested
		if randoOption := request.FormValue("SearchType"); randoOption == "Random" {
			if imageInfo, err := database.DBInterface.GetRandomImage(userQTags); err == nil {
				//redirect user to randomly selected image
				http.Redirect(responseWriter, request, "/image?ID="+strconv.FormatUint(imageInfo.ID,10)+"&prevMessage="+url.QueryEscape(TemplateInput.Message)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), 302);
				return
			}
			TemplateInput.Message += "Failed to search for a random image. " //Just fall through to the normal search
		}
		//Parse tag results for next query
		imageInfo, MaxCount, err := database.DBInterface.SearchImages(userQTags, pageStart, pageStride)
		if err == nil {
			TemplateInput.ImageInfo = imageInfo
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
			logging.LogInterface.WriteLog("ImageRouter", "ImageQueryRouter", "*", "ERROR", []string{"Failed to search images", userQuery, parsed, err.Error()})
		}
	} else {
		logging.LogInterface.WriteLog("ImageRouter", "ImageQueryRouter", "*", "ERROR", []string{"Failed to validate tags", userQuery, err.Error()})
	}

	TemplateInput.Tags = userQTags

	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(userQuery), "/images")

	replyWithTemplate("imageresults.html", TemplateInput, responseWriter)
}

//generatePageMenu generates a template.HTML menu given a few numbers. Returns a menu like "<< 1, 2, 3, [4], 5, 6, 7 >>"
func generatePageMenu(Offset int64, Stride int64, Max int64, Query string, PageURL string) (template.HTML, error) {
	//Validate parameters
	if Offset < 0 || Stride <= 0 || Max < 0 || Offset > Max {
		return template.HTML(""), errors.New("parameters don't make sense. validate your parameters are positive numbers")
	}
	if Max == 0 {
		return template.HTML("1"), nil
	}

	//<a href="/images?SearchTerms={{.OldQuery}}">&#x3C;&#x3C;</a> 1, <a href="#">2</a>, <a href="#">3</a>, <a href="#">4</a>, <a href="#">5</a>, <a href="#">6</a>, <a href="#">7</a> <a href="#">&#x3E;&#x3E;</a>
	ToReturn := "<a href=\"" + PageURL + "?" + Query + "\">&#x3C;&#x3C;</a>"
	//Max possible page number
	maxPage := int64(math.Ceil(float64(Max) / float64(Stride)))
	lastPage := maxPage
	//Current page number
	currentPage := int64(math.Floor(float64(Offset)/float64(Stride)) + 1)
	//Minimum page number we will show
	minPage := currentPage - 3
	if minPage < 1 {
		minPage = 1
	}
	if maxPage > currentPage+3 {
		maxPage = currentPage + 3
	}

	for processPage := minPage; processPage <= maxPage; processPage++ {
		if processPage != currentPage {
			ToReturn = ToReturn + ", <a href=\"" + PageURL + "?" + Query + "&PageStart=" + strconv.FormatInt((processPage-1)*Stride, 10) + "\">" + strconv.FormatInt(processPage, 10) + "</a>"
		} else {
			ToReturn = ToReturn + ", " + strconv.FormatInt(currentPage, 10)
		}
	}

	//Add end
	endOffset := strconv.FormatInt((lastPage-1)*Stride, 10)
	ToReturn = ToReturn + ", <a href=\"" + PageURL + "?" + Query + "&PageStart=" + endOffset + "\">&#x3E;&#x3E;</a>"
	return template.HTML(ToReturn), nil
}

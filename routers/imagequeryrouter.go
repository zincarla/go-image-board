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
	"strconv"
)

//ImageQueryRouter serves requests to /images
func ImageQueryRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	userQuery := TemplateInput.OldQuery

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

	//Get the page offset
	pageStartS := request.FormValue("PageStart")
	upageStart, err := strconv.ParseUint(pageStartS, 10, 32)
	var pageStart uint64
	pageStride := config.Configuration.PageStride
	if err == nil {
		//default to 0 on err
		pageStart = upageStart
	}

	//Cleanup and format tags for use with SearchImages
	userQTags, err := database.DBInterface.GetQueryTags(userQuery, false)
	if err == nil {
		//if signed in, add user's global filters to query
		if TemplateInput.UserInformation.Name != "" {
			userFilterTags, err := database.DBInterface.GetUserFilterTags(TemplateInput.UserInformation.ID, false)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "imagequeryrouter/ImageQueryRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
				TemplateInput.HTMLMessage += template.HTML("Failed to add your global filter to this query. Internal error.<br>")
			} else {
				userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
			}
		}
		//Return random image if requested
		if request.FormValue("SearchType") == "Random" || TemplateInput.ViewMode == "slideshow" {
			imageInfo, err := database.DBInterface.GetRandomImage(userQTags)
			if err == nil {
				//redirect user to randomly selected image
				http.Redirect(responseWriter, request, "/image?ID="+strconv.FormatUint(imageInfo.ID, 10)+"&SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), 302)
				return
			}
			logging.WriteLog(logging.LogLevelError, "imagequeryrouter/ImageQueryRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to search random image", userQuery, err.Error()})
			TemplateInput.HTMLMessage += template.HTML("Failed to search for a random image.<br>") //Just fall through to the normal search
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
			logging.WriteLog(logging.LogLevelError, "imagequeryrouter/ImageQueryRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to search images", userQuery, parsed, err.Error()})
		}
	} else {
		logging.WriteLog(logging.LogLevelError, "imagequeryrouter/ImageQueryRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to validate tags", userQuery, err.Error()})
	}

	TemplateInput.Tags = userQTags

	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(userQuery), "/images")

	replyWithTemplate("imageresults.html", TemplateInput, responseWriter, request)
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

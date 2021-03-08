package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

//TagsRouter serves requests to /tags (Big tag list)
func TagsRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	TemplateInput.TotalResults = 0

	TagSearch := strings.TrimSpace(request.FormValue("SearchTags"))

	//Get the page offset
	pageStartS := request.FormValue("PageStart")
	pageStart, _ := strconv.ParseUint(pageStartS, 10, 32) // Defaults to 0 on error, which is fine
	pageStride := config.Configuration.PageStride

	//Populate Tags
	tag, totalResults, err := database.DBInterface.SearchTags(TagSearch, pageStart, pageStride, false, false)
	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Error pulling tags.<br>")
		logging.WriteLog(logging.LogLevelError, "tagrouter/TagsRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to pull tags ", err.Error()})
	} else {
		TemplateInput.Tags = tag
		TemplateInput.TotalResults = totalResults
	}

	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTags="+url.QueryEscape(TagSearch), "/tags")

	replyWithTemplate("tags.html", TemplateInput, responseWriter, request)
}

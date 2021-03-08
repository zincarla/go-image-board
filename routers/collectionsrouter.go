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

//CollectionsRouter serves requests to /collections
func CollectionsRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	TemplateInput.TotalResults = 0

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
		if TemplateInput.IsLoggedOn() {
			userFilterTags, err := database.DBInterface.GetUserFilterTags(TemplateInput.UserInformation.ID, true)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "collectionqueryrouter/CollectionsRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
				TemplateInput.HTMLMessage += template.HTML("Failed to add your global filter to this query. Internal error.<br>")
			} else {
				userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
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
			logging.WriteLog(logging.LogLevelError, "CollectionQueryRouter/CollectionsRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to search images", TemplateInput.OldQuery, parsed, err.Error()})
		}
	} else {
		logging.WriteLog(logging.LogLevelError, "CollectionQueryRouter/CollectionsRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Failed to validate tags", TemplateInput.OldQuery, err.Error()})
	}

	TemplateInput.Tags = userQTags
	TemplateInput.PageMenu, err = generatePageMenu(int64(pageStart), int64(pageStride), int64(TemplateInput.TotalResults), "SearchTerms="+url.QueryEscape(TemplateInput.OldQuery), "/collections")

	replyWithTemplate("collections.html", TemplateInput, responseWriter, request)
}

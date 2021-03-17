package api

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"strconv"
	"strings"
)

//ImageSearchResult response format for an image search
type ImageSearchResult struct {
	Images       []interfaces.ImageInformation
	ResultCount  uint64
	ServerStride uint64
}

//ImagesGetAPIRouter serves requests to /api/Images
func ImagesGetAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, UserID, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//Query for a images's information, will return ImageInformation
	userQuery := request.FormValue("SearchQuery")
	pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
	pageStride := config.Configuration.PageStride

	userQTags, err := database.DBInterface.GetQueryTags(userQuery, false)
	if err == nil {
		//add user's global filters to query
		userFilterTags, err := database.DBInterface.GetUserFilterTags(UserID, false)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "imagequeries/ImagesAPIRouter", UserName, logging.ResultFailure, []string{"Failed to load user's filter", err.Error()})
		} else {
			userQTags = interfaces.RemoveDuplicateTags(append(userQTags, userFilterTags...))
		}

		//Return random image if requested
		if strings.ToLower(request.FormValue("SearchType")) == "random" {
			imageInfo, resultCount, err := database.DBInterface.GetRandomImage(userQTags)
			if err == nil {
				ReplyWithJSON(responseWriter, request, ImageSearchResult{Images: []interfaces.ImageInformation{imageInfo}, ResultCount: resultCount, ServerStride: pageStride}, UserName)
				return
			}
			logging.WriteLog(logging.LogLevelError, "imagequeries/ImagesAPIRouter", UserName, logging.ResultFailure, []string{"Failed to perform random query", err.Error()})
			ReplyWithJSONError(responseWriter, request, "failed query", UserName, http.StatusInternalServerError)
			return
		}

		//Perform Query
		imageInfo, MaxCount, err := database.DBInterface.SearchImages(userQTags, pageStart, pageStride)
		ReplyWithJSON(responseWriter, request, ImageSearchResult{Images: imageInfo, ResultCount: MaxCount, ServerStride: pageStride}, UserName)
		return
	}
	logging.WriteLog(logging.LogLevelError, "imagequeries/ImagesAPIRouter", UserName, logging.ResultFailure, []string{"Failed to parse user query", err.Error()})
	ReplyWithJSONError(responseWriter, request, "failed to parse your query", UserName, http.StatusInternalServerError)
}

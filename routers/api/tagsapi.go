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

//TagSearchResult response format for a tag search
type TagSearchResult struct {
	Tags         []interfaces.TagInformation
	ResultCount  uint64
	ServerStride uint64
}

//TagsGetAPIRouter serves get requests to /api/Tags
func TagsGetAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User either not logged in, or hit by throttle. Either way, already handled.
	}

	//Query for a tag's information, will return TagInformation
	requestedName := strings.TrimSpace(request.FormValue("tagNameQuery"))
	pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
	pageStride := config.Configuration.PageStride

	//Perform Query
	tagInfo, count, err := database.DBInterface.SearchTags(requestedName, pageStart, pageStride, false, false)
	if err != nil {
		logging.WriteLog(logging.LogLevelError, "tagqueries/TagsAPIRouter", UserName, logging.ResultFailure, []string{"Failed to query tags", err.Error()})
		ReplyWithJSONError(responseWriter, request, "Internal Database Error Occured", UserName, http.StatusInternalServerError)
		return
	}

	ReplyWithJSON(responseWriter, request, TagSearchResult{Tags: tagInfo, ResultCount: count, ServerStride: pageStride}, "")
}

package api

import (
	"go-image-board/database"
	"go-image-board/logging"
	"net/http"
	"strings"
)

//TagNameAPIRouter serves requests to /api/TagName
func TagNameAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//Query for a tag's informaion, will return TagInformation
	requestedName := strings.TrimSpace(request.FormValue("tagNameQuery"))

	//Perform Query
	tagInfo, count, err := database.DBInterface.SearchTags(requestedName, 0, 5, true, true)
	if err != nil {
		logging.WriteLog(logging.LogLevelError, "tagqueries/TagNameAPIRouter", UserName, logging.ResultFailure, []string{"Failed to query tags", err.Error()})
		ReplyWithJSONError(responseWriter, request, "Internal Database Error Occured", UserName, http.StatusInternalServerError)
		return
	}

	ReplyWithJSON(responseWriter, request, TagSearchResult{Tags: tagInfo, ResultCount: count, ServerStride: 5}, "")
}

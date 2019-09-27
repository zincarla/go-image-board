package api

import (
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"strings"
)

//TagSearchResult contains information for a user
type TagSearchResult struct {
	Tags        []interfaces.TagInformation
	ResultCount  uint64
	ServerStride uint64
}

//TagNameAPIRouter serves requests to /api/TagName
func TagNameAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method == "GET" {
		//Query for a collection's information, will return CollectionInformation
		requestedName := strings.TrimSpace(request.FormValue("tagNameQuery"))

		//Perform Query
		tagInfo, count, err := database.DBInterface.SearchTags(requestedName, 0, 5, true, true)
		if err != nil {
			logging.LogInterface.WriteLog("UserQueryAPI", "UserAPIRouter", "*", "ERROR", []string{"Failed to query tags", err.Error()})
			http.Error(responseWriter, "SQL Error", http.StatusInternalServerError)
			return
		}

		ReplyWithJSON(responseWriter, request, TagSearchResult{Tags: tagInfo, ResultCount: count, ServerStride: 5}, "*")
	}
}

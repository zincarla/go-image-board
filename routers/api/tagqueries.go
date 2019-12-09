package api

import (
	"database/sql"
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

//TagNameAPIRouter serves requests to /api/TagName
func TagNameAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//This is used for auto-complete functionality
	if request.Method == "GET" {
		//Query for a tag's informaion, will return TagInformation
		requestedName := strings.TrimSpace(request.FormValue("tagNameQuery"))

		//Perform Query
		tagInfo, count, err := database.DBInterface.SearchTags(requestedName, 0, 5, true, true)
		if err != nil {
			logging.LogInterface.WriteLog("TagQueries", "TagNameAPIRouter", "*", "ERROR", []string{"Failed to query tags", err.Error()})
			ReplyWithJSONError(responseWriter, request, "Internal Database Error Occured", UserName, http.StatusInternalServerError)
			return
		}

		ReplyWithJSON(responseWriter, request, TagSearchResult{Tags: tagInfo, ResultCount: count, ServerStride: 5}, "*")
	}
}

//TagAPIRouter serves requests to /api/Tag
func TagAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User not logged in and was already handled
	}

	//This is used for auto-complete functionality
	if request.Method == "GET" {
		//Query for a tag's informaion, will return TagInformation
		requestedID := request.FormValue("TagID")
		requestedName := request.FormValue("TagName")

		if requestedID != "" {
			//Grab specific tag by ID
			parsedID, err := strconv.ParseUint(requestedID, 10, 32)
			if err != nil {
				ReplyWithJSONError(responseWriter, request, "TagID could not be parsed into a number", UserName, http.StatusBadRequest)
				return
			}
			tag, err := database.DBInterface.GetTag(parsedID)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No tag by that ID", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Internal database error", UserName, http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, tag, UserName)
		} else if requestedName != "" {
			tag, err := database.DBInterface.GetTagByName(requestedName)
			if err != nil {
				if err == sql.ErrNoRows {
					ReplyWithJSONError(responseWriter, request, "No tag by that Name", UserName, http.StatusNotFound)
					return
				}
				ReplyWithJSONError(responseWriter, request, "Internal database error", UserName, http.StatusInternalServerError)
				return
			}
			ReplyWithJSON(responseWriter, request, tag, UserName)
		} else {
			ReplyWithJSONError(responseWriter, request, "Please specify either TagID or TagName", UserName, http.StatusBadRequest)
			return
		}
	}
}

//TagsAPIRouter serves requests to /api/Tags
func TagsAPIRouter(responseWriter http.ResponseWriter, request *http.Request) {
	//Validate Logon
	UserAPIValidated, _, UserName := ValidateAndThrottleAPIUser(responseWriter, request)
	if !UserAPIValidated {
		return //User either not logged in, or hit by throttle. Either way, already handled.
	}

	//This is used for auto-complete functionality
	if request.Method == "GET" {
		//Query for a tag's informaion, will return TagInformation
		requestedName := strings.TrimSpace(request.FormValue("tagNameQuery"))
		pageStart, _ := strconv.ParseUint(request.FormValue("PageStart"), 10, 32) //Either parses fine, or is 0, both works
		pageStride := config.Configuration.PageStride

		//Perform Query
		tagInfo, count, err := database.DBInterface.SearchTags(requestedName, pageStart, pageStride, false, false)
		if err != nil {
			logging.LogInterface.WriteLog("TagQueries", "TagsAPIRouter", "*", "ERROR", []string{"Failed to query tags", err.Error()})
			ReplyWithJSONError(responseWriter, request, "Internal Database Error Occured", UserName, http.StatusInternalServerError)
			return
		}

		ReplyWithJSON(responseWriter, request, TagSearchResult{Tags: tagInfo, ResultCount: count, ServerStride: pageStride}, "*")
	}
}

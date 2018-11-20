package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers/templatecache"
	"html/template"
	"net/http"
	"strings"
	"time"
)

type templateInput struct {
	PageTitle             string
	GIBVersion            string
	ImageInfo             []interfaces.ImageInformation
	CollectionInfo        interfaces.CollectionInformation
	CollectionInfoList    []interfaces.CollectionInformation
	OldQuery              string
	PageMenu              template.HTML
	TotalResults          uint64
	Tags                  []interfaces.TagInformation
	ImageContent          template.HTML
	ImageContentInfo      interfaces.ImageInformation
	TagContentInfo        interfaces.TagInformation
	AliasTagInfo          interfaces.TagInformation
	UserName              string
	UserID                uint64
	Message               string
	AllowAccountCreation  bool
	AccountRequiredToView bool
	QuestionOne           string
	QuestionTwo           string
	QuestionThree         string
	UserPermissions       interfaces.UserPermission
	UserControlsOwn       bool
	RedirectLink          string
	UserFilter            string
	//PreviousMemberID When in a single image view, this should be set to the ID of the previous image in the search (For prev button)
	PreviousMemberID uint64
	//NextMemberID When in a single image view, this should be set to the ID of the next image in the search (For next button)
	NextMemberID uint64
	//StreamView changes view from icons to pages of full content
	StreamView bool
	//RequestStart is start time for a user request
	RequestStart time.Time
	//RequestTime is time it took to process a user request in MS
	RequestTime int64
}

var totalImages uint64
var totalCacheTime time.Time

func replyWithTemplate(templateName string, templateInputInterface interface{}, responseWriter http.ResponseWriter) {
	//Call Template
	templateToUse := templatecache.TemplateCache
	if ti, ok := templateInputInterface.(templateInput); ok {
		ti.RequestTime = time.Now().Sub(ti.RequestStart).Nanoseconds() / 1000000 //Nanosecond to Millisecond
		templateInputInterface = ti
	}
	err := templateToUse.ExecuteTemplate(responseWriter, templateName, templateInputInterface)
	if err != nil {
		logging.LogInterface.WriteLog("routertemplate", "replyWithTemplate", "*", "ERROR", []string{"Parse Error", err.Error()})
		http.Error(responseWriter, "", http.StatusInternalServerError)
		return
	}
}

//ValidateUserLogon Returns either the UserID,Name,Token or 0,"",""
func ValidateUserLogon(request *http.Request) (uint64, string, string) {
	//Verify user is logged in by validating token
	userNameT, tokenIDT, _ := getSessionInformation(request) //This bit actually validates, returns "","" otherwise
	if tokenIDT != "" && userNameT != "" {
		//Translate UserID
		userID, err := database.DBInterface.GetUserID(userNameT)
		if err == nil {
			return userID, userNameT, tokenIDT
		}
		logging.LogInterface.WriteLog("routertemplate", "getNewTemplateInput", userNameT, "ERROR", []string{"Failed to get UserID: ", err.Error()})
	}
	return 0, "", ""
}

//getNewTemplateInput helper function initiliazes a new templateInput with common information
func getNewTemplateInput(request *http.Request) templateInput {
	TemplateInput := templateInput{PageTitle: "GIB",
		GIBVersion:            config.ApplicationVersion,
		AllowAccountCreation:  config.Configuration.AllowAccountCreation,
		UserControlsOwn:       config.Configuration.UsersControlOwnObjects,
		AccountRequiredToView: config.Configuration.AccountRequiredToView,
		RequestStart:          time.Now()}

	//Verify user is logged in by validating token
	userNameT, tokenIDT, session := getSessionInformation(request)
	if tokenIDT != "" && userNameT != "" {
		TemplateInput.UserName = userNameT
		permissions, _ := database.DBInterface.GetUserPermissionSet(userNameT)
		TemplateInput.UserPermissions = interfaces.UserPermission(permissions)
		//Translate UserID
		userID, err := database.DBInterface.GetUserID(userNameT)
		if err == nil {
			TemplateInput.UserID = userID
		} else {
			logging.LogInterface.WriteLog("routertemplate", "getNewTemplateInput", userNameT, "ERROR", []string{"Failed to get UserID: ", err.Error()})
		}
	}

	//Keep view preference
	sessionStreamView, isOk := session.Values["StreamView"].(string)
	if isOk && strings.ToLower(sessionStreamView) == "true" {
		TemplateInput.StreamView = true
	}

	//Grab user query information
	userQuery := strings.ToLower(request.FormValue("SearchTerms"))
	TemplateInput.OldQuery = userQuery

	TemplateInput.Message = request.FormValue("prevMessage")

	//Add total images from server
	if time.Since(totalCacheTime).Hours() > 1 {
		//Get new total
		var err error
		_, totalImages, err = database.DBInterface.SearchImages(make([]interfaces.TagInformation, 0), 0, 1)
		//Don't really care if this has a transient issue
		if err != nil {
			logging.LogInterface.WriteLog("routertemplate", "getNewTemplateInput", "*", "WARNING", []string{"failed to update count cache", err.Error()})
		}
		totalCacheTime = time.Now()
	}
	TemplateInput.TotalResults = totalImages

	return TemplateInput
}

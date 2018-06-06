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
	PageTitle            string
	GIBVersion           string
	ImageInfo            []interfaces.ImageInformation
	OldQuery             string
	PageMenu             template.HTML
	TotalResults         uint64
	Tags                 []interfaces.TagInformation
	ImageContent         template.HTML
	ImageContentInfo     interfaces.ImageInformation
	TagContentInfo       interfaces.TagInformation
	AliasTagInfo         interfaces.TagInformation
	UserName             string
	UserID               uint64
	Message              string
	AllowAccountCreation bool
	QuestionOne          string
	QuestionTwo          string
	QuestionThree        string
	UserPermissions      interfaces.UserPermission
	UserControlsOwn      bool
	RedirectLink         string
	UserFilter           string
}

var totalImages uint64
var totalCacheTime time.Time

func replyWithTemplate(templateName string, templateInput interface{}, responseWriter http.ResponseWriter) {
	//Call Template
	templateToUse := templatecache.TemplateCache
	err := templateToUse.ExecuteTemplate(responseWriter, templateName, templateInput)
	if err != nil {
		logging.LogInterface.WriteLog("routertemplate", "replyWithTemplate", "*", "ERROR", []string{"Parse Error", err.Error()})
		http.Error(responseWriter, "", http.StatusInternalServerError)
		return
	}
}

//getNewTemplateInput helper function initiliazes a new templateInput with common information
func getNewTemplateInput(request *http.Request) templateInput {
	TemplateInput := templateInput{PageTitle: "GIB",
		GIBVersion:           config.ApplicationVersion,
		AllowAccountCreation: config.Configuration.AllowAccountCreation,
		UserControlsOwn:      config.Configuration.UsersControlOwnObjects}

	//Verify user is logged in by validating token
	userNameT, tokenIDT, _ := getSessionInformation(request)
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

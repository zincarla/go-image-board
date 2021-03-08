package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/routers/templatecache"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/csrf"
)

type templateInput struct {
	PageTitle          string
	GIBVersion         string
	ImageInfo          []interfaces.ImageInformation
	CollectionInfo     interfaces.CollectionInformation
	CollectionInfoList []interfaces.CollectionInformation
	OldQuery           string
	PageMenu           template.HTML
	TotalResults       uint64
	Tags               []interfaces.TagInformation
	ImageContent       template.HTML
	ImageContentInfo   interfaces.ImageInformation
	TagContentInfo     interfaces.TagInformation
	AliasTagInfo       interfaces.TagInformation
	//UserName              string
	//UserID                uint64
	UserInformation       interfaces.UserInformation
	Message               string
	HTMLMessage           template.HTML
	AllowAccountCreation  bool
	AccountRequiredToView bool
	QuestionOne           string
	QuestionTwo           string
	QuestionThree         string
	UserPermissions       interfaces.UserPermission
	UserControlsOwn       bool
	RedirectLink          string
	UserFilter            string
	SimilarCount          uint64
	CSRF                  template.HTML
	//PreviousMemberID When in a single image view, this should be set to the ID of the previous image in the search (For prev button)
	PreviousMemberID uint64
	//NextMemberID When in a single image view, this should be set to the ID of the next image in the search (For next button)
	NextMemberID uint64
	//ViewMode changes view mode, can be either "" for normal, stream, to view full images in stream, or slideshow for a auto-playing slideshow
	ViewMode string
	//SlideShowSpeed controls speed of slideshow in seconds
	SlideShowSpeed int64
	//RequestStart is start time for a user request
	RequestStart time.Time
	//RequestTime is time it took to process a user request in MS
	RequestTime int64
	//ModUserData contains information for the modUser page
	ModUserData interfaces.UserInformation
}

func (ti templateInput) IsLoggedOn() bool {
	return ti.UserInformation.ID != 0 && ti.UserInformation.Name != ""
}

var totalImages uint64
var totalCacheTime time.Time

func replyWithTemplate(templateName string, templateInputInterface interface{}, responseWriter http.ResponseWriter, request *http.Request) {
	//Call Template
	templateToUse := templatecache.TemplateCache
	if ti, ok := templateInputInterface.(templateInput); ok {
		ti.RequestTime = time.Now().Sub(ti.RequestStart).Nanoseconds() / 1000000 //Nanosecond to Millisecond
		applyFlash(responseWriter, request, &ti)                                 //Apply any pending flash cookies
		templateInputInterface = ti
	}
	err := templateToUse.ExecuteTemplate(responseWriter, templateName, templateInputInterface)
	if err != nil {
		logging.WriteLog(logging.LogLevelError, "routertemplate/replyWithTemplate", "0", logging.ResultFailure, []string{"Parse Error", err.Error()})
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
		logging.WriteLog(logging.LogLevelWarning, "routertemplate/ValidateUserLogon", userNameT, logging.ResultFailure, []string{"Failed to get UserID: ", err.Error()})
	}
	return 0, "", ""
}

//getNewTemplateInput helper function initiliazes a new templateInput with common information
func getNewTemplateInput(responseWriter http.ResponseWriter, request *http.Request) templateInput {
	TemplateInput := templateInput{PageTitle: "GIB",
		GIBVersion:            config.ApplicationVersion,
		AllowAccountCreation:  config.Configuration.AllowAccountCreation,
		UserControlsOwn:       config.Configuration.UsersControlOwnObjects,
		AccountRequiredToView: config.Configuration.AccountRequiredToView,
		RequestStart:          time.Now(),
		CSRF:                  csrf.TemplateField(request),
		UserInformation:       interfaces.UserInformation{}}

	//Verify user is logged in by validating token
	userNameT, tokenIDT, session := getSessionInformation(request)
	if tokenIDT != "" && userNameT != "" {
		permissions, _ := database.DBInterface.GetUserPermissionSet(userNameT)
		TemplateInput.UserPermissions = interfaces.UserPermission(permissions)
		//Translate UserID
		userID, err := database.DBInterface.GetUserID(userNameT)
		if err == nil {
			TemplateInput.UserInformation.ID = userID
			TemplateInput.UserInformation.Name = userNameT
		} else {
			logging.WriteLog(logging.LogLevelError, "routertemplate/getNewTemplateInput", userNameT, logging.ResultFailure, []string{"Failed to get UserID: ", err.Error()})
		}
	}

	//Add IP to user info
	var err error
	TemplateInput.UserInformation.IP, _, err = net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		TemplateInput.UserInformation.IP = request.RemoteAddr
	}

	//Keep view preference
	sessionViewMode, isOk := session.Values["ViewMode"].(string)
	if isOk && strings.ToLower(sessionViewMode) == "stream" {
		TemplateInput.ViewMode = "stream"
	} else if isOk && strings.ToLower(sessionViewMode) == "slideshow" {
		TemplateInput.ViewMode = "slideshow"
	} else {
		TemplateInput.ViewMode = "grid"
	}

	slideShowSpeed, isOk := session.Values["slideshowspeed"].(int64)
	if isOk {
		TemplateInput.SlideShowSpeed = slideShowSpeed
	} else {
		TemplateInput.SlideShowSpeed = 30
	}

	//Grab user query information
	userQuery := strings.ToLower(request.FormValue("SearchTerms"))
	TemplateInput.OldQuery = userQuery

	//Add total images from server
	if time.Since(totalCacheTime).Hours() > 1 {
		//Get new total
		var err error
		_, totalImages, err = database.DBInterface.SearchImages(make([]interfaces.TagInformation, 0), 0, 1)
		//Don't really care if this has a transient issue
		if err != nil {
			logging.WriteLog(logging.LogLevelWarning, "routertemplate/getNewTemplateInput", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"failed to update count cache", err.Error()})
		}
		totalCacheTime = time.Now()
	}
	TemplateInput.TotalResults = totalImages

	return TemplateInput
}

//getTemplateInputFromRequest helper function gets a TemplateInput that was already generated for request
func getTemplateInputFromRequest(responseWriter http.ResponseWriter, request *http.Request) templateInput {
	TemplateInput, ok := request.Context().Value(TemplateInputKeyID).(templateInput)
	if ok {
		return TemplateInput
	}
	logging.WriteLog(logging.LogLevelError, "routertemplate/getTemplateInputFromRequest", "0", logging.ResultFailure, []string{"Failed to get TemplateInput from request"})
	return getNewTemplateInput(responseWriter, request)
}

//applyFlash checks for flash cookies, and applies them to template
func applyFlash(responseWriter http.ResponseWriter, request *http.Request, TemplateInput *templateInput) {
	if request.FormValue("flash") == "" {
		return
	}
	session, err := config.SessionStore.Get(request, config.SessionVariableName)
	if err == nil {
		//Load flash if necessary
		if request.FormValue("flash") != "" {
			pendingFlashes := session.Flashes(request.FormValue("flash"))
			if len(pendingFlashes) > 0 {
				fullMessage := ""
				for _, pendingFlash := range pendingFlashes {
					if pf, ok := pendingFlash.(string); ok {
						fullMessage += pf + "<br>"
					}
				}
				TemplateInput.HTMLMessage += template.HTML(fullMessage)
				session.Save(request, responseWriter)
			}
		}
	}
}

//createFlash creates a flash cookie and saves the session
func createFlash(responseWriter http.ResponseWriter, request *http.Request, flashMessage string, flashName string) error {
	session, _ := config.SessionStore.Get(request, config.SessionVariableName)
	session.AddFlash(flashMessage, flashName)
	return session.Save(request, responseWriter)
}

//creates a flash cookie, and sends a redirect to the client to the root with the flash cookie message
func redirectWithFlash(responseWriter http.ResponseWriter, request *http.Request, redirectURL string, flashMessage string, flashName string) error {
	err := createFlash(responseWriter, request, flashMessage, flashName)
	toAppend := "?flash=" + flashName
	if strings.ContainsRune(redirectURL, '?') {
		toAppend = "&flash=" + flashName
	}
	http.Redirect(responseWriter, request, redirectURL+toAppend, http.StatusFound)
	return err
}

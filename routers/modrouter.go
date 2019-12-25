package routers

import (
	"go-image-board/config"
	"net/http"
	"net/url"
)

//ModRouter serves requests to /mod
func ModRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	if TemplateInput.UserName == "" && config.Configuration.AccountRequiredToView {
		http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
		return
	}

	replyWithTemplate("mod.html", TemplateInput, responseWriter)
}

package routers

import (
	"net/http"
)

//ModRouter serves requests to /mod
func ModRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	replyWithTemplate("mod.html", TemplateInput, responseWriter, request)
}

package routers

import (
	"go-image-board/config"
	"html/template"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
)

//AboutRouter serves requests to /about, all files in /about should be html, and they will be treated as templates
func AboutRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	urlVariables := mux.Vars(request)

	filePath := config.Configuration.HTTPRoot + string(filepath.Separator) + "about" + string(filepath.Separator) + urlVariables["file"]

	data, err := ioutil.ReadFile(filePath)

	if err != nil {
		TemplateInput.HTMLMessage += template.HTML("Content not found.<br>")
		TemplateInput.ImageContent = "Content not found"
	}
	TemplateInput.ImageContent = template.HTML(string(data))

	replyWithTemplate("about.html", TemplateInput, responseWriter, request)
}

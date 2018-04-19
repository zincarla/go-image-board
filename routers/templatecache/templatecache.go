package templatecache

import (
	"go-image-board/config"
	"go-image-board/logging"
	"html/template"
	"io/ioutil"
	"strings"
)

//TemplateCache contains a cache of templates used by the server
var TemplateCache = template.New("")

//CacheTemplates loads the TemplateCache. This should be called before use
func CacheTemplates() error {
	var allFiles []string
	files, err := ioutil.ReadDir(config.Configuration.HTTPRoot)
	if err != nil {
		logging.LogInterface.WriteLog("templatcache", "CacheTemplates", "*", "ERROR", []string{err.Error()})
		return err
	}
	for _, file := range files {
		filename := file.Name()
		if strings.HasSuffix(filename, ".html") {
			allFiles = append(allFiles, config.JoinPath(config.Configuration.HTTPRoot, filename))
		}
	}

	//Add functions here

	templates, err := template.ParseFiles(allFiles...)
	if err != nil {
		logging.LogInterface.WriteLog("TemplateCache", "CacheTemplates", "*", "ERROR", []string{err.Error()})
		return err
	}
	TemplateCache = templates
	logging.LogInterface.WriteLog("TemplateCache", "CacheTemplates", "*", "INFO", append([]string{"Added Templates"}, allFiles...))
	return nil
}

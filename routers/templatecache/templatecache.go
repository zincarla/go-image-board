package templatecache

import (
	"go-image-board/config"
	"go-image-board/logging"
	"html/template"
	"io/ioutil"
	"path/filepath"
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
	getImageType := func(path string) string {
		text := ""
		switch ext := filepath.Ext(strings.ToLower(path)); ext {
		case ".wav", ".mp3", ".ogg":
			text = "audio"
		case ".mpg", ".mov", ".webm", ".avi", ".mp4", ".gif":
			text = "video"
		default:
			text = "image"
		}
		return text
	}
	templates := template.New("")
	templates = templates.Funcs(template.FuncMap{"getimagetype": getImageType})

	templates, err = templates.ParseFiles(allFiles...)
	if err != nil {
		logging.LogInterface.WriteLog("TemplateCache", "CacheTemplates", "*", "ERROR", []string{err.Error()})
		return err
	}
	TemplateCache = templates
	logging.LogInterface.WriteLog("TemplateCache", "CacheTemplates", "*", "INFO", append([]string{"Added Templates"}, allFiles...))
	return nil
}

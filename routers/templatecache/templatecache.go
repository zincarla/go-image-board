package templatecache

import (
	"fmt"
	"go-image-board/config"
	"go-image-board/logging"
	"html/template"
	"io/ioutil"
	"path/filepath"
	"strconv"
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
	increment := func(value interface{}) interface{} {
		switch value.(type) {
		case int:
			return value.(int) + 1
		case float64:
			return value.(float64) + 1
		case float32:
			return value.(float32) + 1
		case uint64:
			return value.(uint64) + 1
		case uint32:
			return value.(uint32) + 1
		}
		return value
	}
	decrement := func(value interface{}) interface{} {
		switch value.(type) {
		case int:
			return value.(int) - 1
		case float64:
			return value.(float64) - 1
		case float32:
			return value.(float32) - 1
		case uint64:
			return value.(uint64) - 1
		case uint32:
			return value.(uint32) - 1
		}
		return value
	}
	getEmbed := func(value interface{}) template.HTML {
		return GetEmbedForContent(fmt.Sprintf("%v", value))
	}
	templates := template.New("")
	templates = templates.Funcs(template.FuncMap{"getimagetype": getImageType})
	templates = templates.Funcs(template.FuncMap{"inc": increment})
	templates = templates.Funcs(template.FuncMap{"dec": decrement})
	templates = templates.Funcs(template.FuncMap{"getEmbed": getEmbed})

	templates, err = templates.ParseFiles(allFiles...)
	if err != nil {
		logging.LogInterface.WriteLog("TemplateCache", "CacheTemplates", "*", "ERROR", []string{err.Error()})
		return err
	}
	TemplateCache = templates
	logging.LogInterface.WriteLog("TemplateCache", "CacheTemplates", "*", "INFO", []string{"Added Templates", strconv.Itoa(len(allFiles))})
	return nil
}

//GetEmbedForContent Returns the html necessary to embed the specified file
func GetEmbedForContent(imageLocation string) template.HTML {
	ToReturn := ""

	switch ext := filepath.Ext(strings.ToLower(imageLocation)); ext {
	case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".svg", ".webp", ".tiff", ".tif", ".jfif":
		ToReturn = "<img src=\"/images/" + imageLocation + "\" alt=\"" + imageLocation + "\" id=\"IMGContent\" />"
	case ".mpg", ".mov", ".webm", ".avi", ".mp4", ".mp3", ".ogg":
		ToReturn = "<video controls loop> <source src=\"/images/" + imageLocation + "\" type=\"" + getMIME(ext, "video/mp4") + "\">Your browser does not support the video tag.</video>"
	case ".wav":
		ToReturn = "<audio controls loop> <source src=\"/images/" + imageLocation + "\" type=\"" + getMIME(ext, "audio/wav") + "\">Your browser does not support the audio tag.</audio>"
	default:
		logging.LogInterface.WriteLog("ImageRouter", "getEmbedForConent", "*", "WARN", []string{"File uploaded, but did not match a filter during download", imageLocation})
		ToReturn = "<p>File format not supported. Click download.</p>"
	}

	return template.HTML(ToReturn)
}

//returns a mime given a file extension. This is only required for video and audio files so we can embed mime in video/audio element
func getMIME(extension string, fallback string) string {
	switch extension {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/avi"
	case ".mpg":
		return "video/mpeg"
	case ".mov":
		return "video/quicktime"
	case ".ogg":
		return "video/ogg"
	case ".mp3":
		return "audio/mpeg3"
	case ".wav":
		return "audio/wav"
	default:
		return fallback
	}
}

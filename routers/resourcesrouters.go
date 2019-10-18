package routers

import (
	"errors"
	"go-image-board/config"
	"go-image-board/logging"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imageorient"

	//Because all image processing will happen in this file
	_ "image/gif"
	_ "image/jpeg"
	"image/png"

	"github.com/gorilla/mux"
	"github.com/nfnt/resize"

	//Because all image processing will happen in this file
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

//ResourceRouter handles requests to /resources
func ResourceRouter(responseWriter http.ResponseWriter, request *http.Request) {
	urlVariables := mux.Vars(request)
	logging.LogInterface.WriteLog("ContentRouter", "ResourceRouter", "*", "SUCCESS", []string{"resources" + string(filepath.Separator) + urlVariables["file"]})
	http.ServeFile(responseWriter, request, config.JoinPath(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+urlVariables["file"]))
}

//RedirectRouter handles requests to /redirect
func RedirectRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)
	TemplateInput.RedirectLink = request.FormValue("RedirectLink")
	logging.LogInterface.WriteLog("ContentRouter", "RedirectRouter", "*", "INFO", []string{request.FormValue("RedirectLink")})
	replyWithTemplate("redirect.html", TemplateInput, responseWriter)
}

//ResourceImageRouter handles requests to /images/{file}
func ResourceImageRouter(responseWriter http.ResponseWriter, request *http.Request) {
	if config.Configuration.AccountRequiredToView {
		if _, userName, _ := ValidateUserLogon(request); userName == "" {
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
			return
		}
	}
	urlVariables := mux.Vars(request)
	logging.LogInterface.WriteLog("ContentRouter", "ResourceImageRouter", "*", "SUCCESS", []string{config.JoinPath(config.Configuration.ImageDirectory, urlVariables["file"])})
	http.ServeFile(responseWriter, request, config.JoinPath(config.Configuration.ImageDirectory, urlVariables["file"]))
}

//ThumbnailRouter handls requests to /thumbs
func ThumbnailRouter(responseWriter http.ResponseWriter, request *http.Request) {
	if config.Configuration.AccountRequiredToView {
		if _, userName, _ := ValidateUserLogon(request); userName == "" {
			http.Redirect(responseWriter, request, "/logon?prevMessage="+url.QueryEscape("Access to this server requires an account"), 302)
			return
		}
	}
	urlVariables := mux.Vars(request)
	thumbnailPath := config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+urlVariables["file"]+".png")
	//Check if file does not exist
	if _, err := os.Stat(thumbnailPath); err != nil {
		switch ext := filepath.Ext(strings.ToLower(urlVariables["file"])); ext {
		//If it does not, and it is an image, return the original image, more bandwidth but better looking site
		case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".svg", ".webp", ".tiff", ".tif":
			thumbnailPath = config.JoinPath(config.Configuration.ImageDirectory, string(filepath.Separator)+urlVariables["file"])
		//If a video or music file, pull up a play icon
		case ".mpg", ".mov", ".webm", ".avi", ".mp4", ".mp3", ".ogg", ".wav":
			thumbnailPath = config.JoinPath(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"playicon.svg")
		}
	}
	//Final fallback, just return a no image type icon
	if _, err := os.Stat(thumbnailPath); err != nil {
		thumbnailPath = config.JoinPath(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"noicon.svg")
	}

	http.ServeFile(responseWriter, request, thumbnailPath)
}

//GenerateThumbnail will attempt to generate a thumbnail for the specified resource
func GenerateThumbnail(Name string) error {
	//Switch on extension
	//Each case will contain generators for that file type
	//In future for example, could load a random frame from a video file and use that.
	//ATM, just images are supported
	switch ext := filepath.Ext(strings.ToLower(Name)); ext {
	case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".webp", ".tiff", ".tif":
		File, err := os.Open(config.JoinPath(config.Configuration.ImageDirectory, Name))
		defer File.Close()
		if err != nil {
			return err
		}
		originalImage, _, err := imageorient.Decode(File)
		if err != nil {
			return err
		}
		newWidth := uint(originalImage.Bounds().Max.X)
		newHeight := uint(originalImage.Bounds().Max.Y)

		if (newWidth >= newHeight) && newWidth > config.Configuration.MaxThumbnailWidth {
			scale := float64(config.Configuration.MaxThumbnailWidth) / float64(newWidth)
			newWidth = uint(float64(newWidth) * scale)
			newHeight = uint(float64(newHeight) * scale)
		}
		if (newHeight > newWidth) && newHeight > config.Configuration.MaxThumbnailHeight {
			scale := float64(config.Configuration.MaxThumbnailHeight) / float64(newHeight)
			newWidth = uint(float64(newWidth) * scale)
			newHeight = uint(float64(newHeight) * scale)
		}
		//Open the specified file at Path
		NewFile, err := os.OpenFile(config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+Name+".png"), os.O_CREATE|os.O_RDWR, 0660)
		defer NewFile.Close()
		if err != nil {
			return err
		}
		thumbnailImage := resize.Resize(uint(newWidth), uint(newHeight), originalImage, resize.Lanczos3)
		return png.Encode(NewFile, thumbnailImage)
	case ".mpg", ".mov", ".webm", ".avi", ".mp4":
		logging.LogInterface.WriteLog("resourcesrouters", "GenerateThumbnail", "*", "DEBUG", []string{"Video detected", Name})

		//Short circuit if can't support with FFMPEG
		if !config.Configuration.UseFFMPEG {
			return errors.New("No thumbnail method for file type")
		}
		//Spawn FFMPEG Process and save image file
		//ffmpeg -i input.mp4 -vf  "thumbnail,scale=640:360" -frames:v 1 thumb.png
		//Fire forget
		sizeParam := "thumbnail,scale=" + strconv.FormatUint(uint64(config.Configuration.MaxThumbnailWidth), 10) + ":" + strconv.FormatUint(uint64(config.Configuration.MaxThumbnailHeight), 10)
		ffmpegCMD := exec.Command(config.Configuration.FFMPEGPath, "-i", config.JoinPath(config.Configuration.ImageDirectory, Name), "-vf", sizeParam, "-frames:v", "1", config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+Name+".png"))
		_, err := ffmpegCMD.Output()
		if err != nil {
			logging.LogInterface.WriteLog("resourcesrouters", "GenerateThumbnail", "*", "ERROR", []string{"Failed to use FFMPEG", Name, err.Error()})
			return err
		}
		logging.LogInterface.WriteLog("resourcesrouters", "GenerateThumbnail", "*", "INFO", []string{"FFMPEG output success", Name})
		return nil
	default:
		return errors.New("No thumbnail method for file type")
	}
}

package routers

import (
	"errors"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"net/http"
	"os"
	"os/exec"
	"path"
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
	http.ServeFile(responseWriter, request, path.Join(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+urlVariables["file"]))
}

//RedirectRouter handles requests to /redirect
func RedirectRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)
	TemplateInput.RedirectLink = request.FormValue("RedirectLink")
	logging.WriteLog(logging.LogLevelVerbose, "resourcesrouters/RedirectRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultInfo, []string{request.FormValue("RedirectLink")})
	replyWithTemplate("redirect.html", TemplateInput, responseWriter, request)
}

//ResourceImageRouter handles requests to /images/{file}
func ResourceImageRouter(responseWriter http.ResponseWriter, request *http.Request) {
	urlVariables := mux.Vars(request)
	http.ServeFile(responseWriter, request, path.Join(config.Configuration.ImageDirectory, urlVariables["file"]))
}

//ThumbnailRouter handls requests to /thumbs
func ThumbnailRouter(responseWriter http.ResponseWriter, request *http.Request) {
	urlVariables := mux.Vars(request)
	thumbnailPath := path.Join(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+urlVariables["file"]+".png")
	//Check if file does not exist
	if _, err := os.Stat(thumbnailPath); err != nil {
		switch ext := filepath.Ext(strings.ToLower(urlVariables["file"])); ext {
		//If it does not, and it is an image, return the original image, more bandwidth but better looking site
		case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".svg", ".webp", ".tiff", ".tif", ".jfif":
			thumbnailPath = path.Join(config.Configuration.ImageDirectory, string(filepath.Separator)+urlVariables["file"])
		//If a video or music file, pull up a play icon
		case ".mpg", ".mov", ".webm", ".avi", ".mp4", ".mp3", ".ogg", ".wav":
			thumbnailPath = path.Join(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"playicon.svg")
		}
	}
	//Final fallback, just return a no image type icon
	if _, err := os.Stat(thumbnailPath); err != nil {
		thumbnailPath = path.Join(config.Configuration.HTTPRoot, "resources"+string(filepath.Separator)+"noicon.svg")
	}

	http.ServeFile(responseWriter, request, thumbnailPath)
}

//GenerateThumbnail will attempt to generate a thumbnail for the specified resource
func GenerateThumbnail(Name string) error {
	//Switch on extension
	//Each case will contain generators for that file type
	switch ext := filepath.Ext(strings.ToLower(Name)); ext {
	case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".webp", ".tiff", ".tif", ".jfif":
		File, err := os.Open(path.Join(config.Configuration.ImageDirectory, Name))
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
		NewFile, err := os.OpenFile(path.Join(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+Name+".png"), os.O_CREATE|os.O_RDWR, 0660)
		defer NewFile.Close()
		if err != nil {
			return err
		}
		thumbnailImage := resize.Resize(uint(newWidth), uint(newHeight), originalImage, resize.Lanczos3)
		return png.Encode(NewFile, thumbnailImage)
	case ".mpg", ".mov", ".webm", ".avi", ".mp4":
		logging.WriteLog(logging.LogLevelDebug, "resourcesrouters/GenerateThumbnail", "0", logging.ResultInfo, []string{"Video detected", Name})

		//Short circuit if can't support with FFMPEG
		if !config.Configuration.UseFFMPEG {
			return errors.New("No thumbnail method for file type")
		}
		//Spawn FFMPEG Process and save image file
		//ffmpeg -i input.mp4 -vf  "thumbnail,scale=640:360" -frames:v 1 thumb.png
		//Fire forget
		sizeParam := "thumbnail,scale=" + strconv.FormatUint(uint64(config.Configuration.MaxThumbnailWidth), 10) + ":" + strconv.FormatUint(uint64(config.Configuration.MaxThumbnailHeight), 10)
		ffmpegCMD := exec.Command(config.Configuration.FFMPEGPath, "-i", path.Join(config.Configuration.ImageDirectory, Name), "-vf", sizeParam, "-frames:v", "1", path.Join(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+Name+".png"))
		_, err := ffmpegCMD.Output()
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "resourcesrouters/GenerateThumbnail", "0", logging.ResultFailure, []string{"Failed to use FFMPEG", Name, err.Error()})
			return err
		}
		logging.WriteLog(logging.LogLevelInfo, "resourcesrouters/GenerateThumbnail", "0", logging.ResultInfo, []string{"FFMPEG output success", Name})
		return nil
	default:
		return errors.New("No thumbnail method for file type")
	}
}

//GeneratedHash will attempt to generate a dHash for the given image
func GeneratedHash(Name string, ImageID uint64) error {
	//Switch on extension
	switch ext := filepath.Ext(strings.ToLower(Name)); ext {
	case ".jpg", ".jpeg", ".bmp", ".gif", ".png", ".webp", ".tiff", ".tif", ".jfif":
		//Load image
		File, err := os.Open(path.Join(config.Configuration.ImageDirectory, Name))
		defer File.Close()
		if err != nil {
			return err
		}
		originalImage, _, err := imageorient.Decode(File)
		if err != nil {
			return err
		}
		//Scale it
		const newWidth = 9
		const newHeight = 9
		originalImage = resize.Resize(uint(newWidth), uint(newHeight), originalImage, resize.Lanczos3)

		//Greyscale it
		greyScaledImage := [newWidth][newHeight]byte{}
		for x := 0; x < newWidth; x++ {
			for y := 0; y < newHeight; y++ {
				r, g, b, _ := originalImage.At(x, y).RGBA()
				greyScaledImage[x][y] = byte((r + g + b) / 3)
			}
		}

		//Now we compute hashes, one vertical and one horizontal
		//Using dHash per instructions at http://www.hackerfactor.com/blog/index.php?/archives/529-Kind-of-Like-That.html
		vHash := uint64(0)
		hHash := uint64(0)
		bitLocation := 64
		for y := 1; y < newHeight; y++ {
			for x := 1; x < newWidth; x++ {
				bitLocation--
				if greyScaledImage[x][y] > greyScaledImage[x-1][y] {
					hHash = hHash | (1 << bitLocation)
				}
			}
		}
		bitLocation = 64
		for x := 1; x < newWidth; x++ {
			for y := 1; y < newHeight; y++ {
				bitLocation--
				if greyScaledImage[x][y] > greyScaledImage[x][y-1] {
					vHash = vHash | (1 << bitLocation)
				}
			}
		}

		return database.DBInterface.SetImagedHash(ImageID, hHash, vHash)
	default:
		return errors.New("Cannot process image of this type")
	}
}

package main

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"go-image-board/routers"
	"os"
	"path/filepath"
	"strconv"
)

func renameAllImages() {
	_, maxCount, err := database.DBInterface.SearchImages(nil, 0, config.Configuration.PageStride)
	if err != nil {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Failed to query for images", err.Error()})
		return
	}
	logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Images to process", strconv.FormatUint(maxCount, 10)})
	//Loop through the images one page at a time
	for count := uint64(0); count < maxCount; count += config.Configuration.PageStride {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Processing at", strconv.FormatUint(count, 10)})
		images, _, err := database.DBInterface.SearchImages(nil, count, config.Configuration.PageStride)
		if err != nil {
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Failed to query for images", err.Error()})
			return
		}
		//Loop through the images in this page
		for _, imageInfo := range images {
			//Open file for reading
			fileStream, err := os.Open(config.JoinPath(config.Configuration.ImageDirectory, imageInfo.Location))
			if err != nil {
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Failed to open file", err.Error()})
				return
			}

			//Get new name
			newName, err := routers.GetNewImageName(imageInfo.Location, fileStream)
			fileStream.Close()
			if err != nil {
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Error generating new name", err.Error()})
				return //On error cancel out to keep db and image in sync
			}
			if newName == imageInfo.Location {
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Skipping due to same name", newName})
				continue //Skip if same name
			}
			//Rename image
			if err := os.Rename(config.JoinPath(config.Configuration.ImageDirectory, imageInfo.Location), config.JoinPath(config.Configuration.ImageDirectory, newName)); err != nil {
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Error renaming file", err.Error()})
				return //On error cancel out to keep db and image in sync
			}
			//Rename thumbnail
			if err := os.Rename(config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+imageInfo.Location+".png"), config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+newName+".png")); err != nil {
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "WARN", []string{"Error renaming file", err.Error()})
			}
			//Update database
			if err := database.DBInterface.UpdateImage(imageInfo.ID, nil, nil, nil, nil, newName); err != nil {
				//Rollback and cancel on error
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Error adding renamed image to db, cancelling", err.Error()})
				//Rename thumbnail
				if err := os.Rename(config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+newName+".png"), config.JoinPath(config.Configuration.ImageDirectory, "thumbs"+string(filepath.Separator)+imageInfo.Location+".png")); err != nil {
					logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "WARN", []string{"Error renaming file", err.Error()})
				}
				//Rename image
				if err := os.Rename(config.JoinPath(config.Configuration.ImageDirectory, newName), config.JoinPath(config.Configuration.ImageDirectory, imageInfo.Location)); err != nil {
					logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Error renaming file", err.Error()})
				}
				return
			}
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Successfull rename", newName})
		}
	}
}

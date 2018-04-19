package interfaces

import (
	"time"
)

//ImageInformation contains information for a specific image. This is used to output search results
type ImageInformation struct {
	ID           uint64
	Name         string
	Location     string
	UploaderID   uint64
	UploaderName string
	UploadTime   time.Time
}

package interfaces

import (
	"time"
)

//CollectionInformation contains information for a specific collection. This is usefull when understanding DB Output
type CollectionInformation struct {
	//Basic information
	Name        string
	Description string
	ID          uint64
	UploaderID  uint64
	UploadTime  time.Time
	//Members Number of members in this collection
	Members uint64
	//Special for images
	//OrderInCollection When in a single image view, this shows how far into a collection the image is (ex, 4/50)
	OrderInCollection uint64
	//PreviousMemberID When in a single image view, this should be set to the ID of the previous image in the collection (For prev button)
	PreviousMemberID uint64
	//NextMemberID When in a single image view, this should be set to the ID of the next image in the collection (For next button)
	NextMemberID uint64
	//Location When in a collection list, this should be set to the name/location of a preview image (Same as imageinformation)
	Location string
}

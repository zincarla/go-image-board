package interfaces

import (
	"time"
)

//ImageInformation contains information for a specific image. This is used to output search results
type ImageInformation struct {
	ID              uint64
	Name            string
	Location        string
	Description     string
	UploaderID      uint64
	UploaderName    string
	UploadTime      time.Time
	Rating          string
	ScoreAverage    int64
	ScoreTotal      int64
	ScoreVoters     int64
	UsersVotedScore int64
	Source          string
	SourceIsURL     bool
	//Special for collections
	OrderInCollection uint64                  //Should be used in overview of a single collection
	MemberCollections []CollectionInformation //Should be used in view of single image (For navigation of collections it's a member of)
}

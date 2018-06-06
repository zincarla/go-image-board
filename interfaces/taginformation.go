package interfaces

import (
	"time"
)

//TagInformation contains information for a specific tag. This is usefull when understanding DB Output
type TagInformation struct {
	//Basic information
	Name        string
	Description string
	ID          uint64
	UploaderID  uint64
	UploadTime  time.Time
	AliasedID   uint64
	IsAlias     bool
	//If the tag is a valid tag
	Exists bool
	//If user is trying to exclude this tag/value
	Exclude bool
	//Is a meta tag, not a user tag
	IsMeta bool
	//Value for metatag
	MetaValue interface{}
	//Comparator for meta tag (=,>,<,<=,>=)
	Comparator string
	//Is this metatag special (Not a direct property of an image)
	IsComplexMeta bool
	//Is this tag being added due to a user's global filter
	FromUserFilter bool
}

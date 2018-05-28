package interfaces

import (
	"time"
)

//TagInformation contains information for a specific tag. This is usefull when understanding DB Output
type TagInformation struct {
	Name        string
	Description string
	ID          uint64
	Exists      bool
	Exclude     bool
	UploaderID  uint64
	UploadTime  time.Time
	AliasedID   uint64
	IsAlias     bool
	IsMeta      bool
	MetaValue   interface{}
	Comparator  string
}

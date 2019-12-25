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
	UseCount    uint64
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

//RemoveDuplicateTags removes duplicate tags from a given TagInformation slice.
//This should be used whenever joining two slices of TagInformation. Duplicate tags do not work well in queries
//Example, without this, if a user searched "test" and their account had a global filter of "test", the SQL query would look for two instances of "test"
//This does not work as an image should only have 1 instance of the tag. This situation returns empty results.
func RemoveDuplicateTags(ToFilter []TagInformation) []TagInformation {
	//Rules:
	//We do not care about meta-tags
	//Exlusionary tags win

	for Index := 0; Index < len(ToFilter); Index++ {
		if ToFilter[Index].IsMeta {
			continue //Skip Metatags
		}
		//Standard tag confirmed, scan for duplicates
		for ScanIndex := 0; ScanIndex < len(ToFilter); ScanIndex++ {
			if Index == ScanIndex || ToFilter[ScanIndex].IsMeta {
				continue //Skip comparing same entry, or meta tags
			}
			if ToFilter[ScanIndex].ID == ToFilter[Index].ID {
				var ToRemove int
				if ToFilter[ScanIndex].Exclude {
					//Duplicate found is an exclusionary, so remove Index
					ToRemove = Index
				} else {
					//Duplicate found is not exclusion, so remove ScanIndex
					ToRemove = ScanIndex
				}
				//Remove and resize
				ToFilter = append(ToFilter[:ToRemove], ToFilter[ToRemove+1:]...)
				if ToRemove < Index {
					//If we removed something before the index, then continue scan but decrement the current scan state
					Index--
					ScanIndex--
				} else if ToRemove == Index {
					//If we removed the current index, the decrement index, and start a new duplicate scan from whatever is there now
					Index--
					break
				} else {
					//Finally, the third potential, is we removed an element ahead of Index, in which case, we just need to continue current scan from the same ScanIndex
					ScanIndex--
				}
			}
		}
	}

	return ToFilter
}

//MergeTagSlices merges two tag slices, the second slice will always win for duplicates.
func MergeTagSlices(Original []TagInformation, ToAdd []TagInformation) []TagInformation {
	//Rules:
	//We do not care about meta-tags
	//Tags in ToAdd win
	//Exlusionary tags win after tags in ToAdd

	//First, remove duplicates from original that exist in ToAdd
	for Index := 0; Index < len(ToAdd); Index++ {
		if ToAdd[Index].IsMeta {
			continue //Skip Metatags
		}
		//Standard tag confirmed, scan for duplicates
		for ScanIndex := 0; ScanIndex < len(Original); ScanIndex++ {
			if Original[ScanIndex].IsMeta {
				continue //Skip comparing metas
			}
			if Original[ScanIndex].ID == ToAdd[Index].ID {
				//Remove and resize
				Original = append(Original[:ScanIndex], Original[ScanIndex+1:]...)
				//we just need to continue current scan from the same ScanIndex
				ScanIndex--
			}
		}
	}

	//Now we can fall back to RemoveDuplicateTags to cleanup any other issues
	return RemoveDuplicateTags(append(Original, ToAdd...))
}

package interfaces

import (
	"strconv"
	"time"
)

//UserInformation contains information for a user
type UserInformation struct {
	ID           uint64
	Name         string
	CreationTime time.Time
	Permissions  UserPermission
	Disabled     bool
	IP           string
}

//GetCompositeID This returns a string of identifiers for the user
func (ui UserInformation) GetCompositeID() string {
	toReturn := ""
	if ui.Name != "" && ui.ID != 0 {
		toReturn += ui.Name + "/" + strconv.FormatUint(ui.ID, 10) + " "
	} else {
		toReturn += "- "
	}
	if ui.IP != "" {
		toReturn += ui.IP + " "
	} else {
		toReturn += "- "
	}
	return toReturn[:len(toReturn)-1]
}

package interfaces

import (
	"time"
)

//UserInformation contains information for a user
type UserInformation struct {
	ID           uint64
	Name         string
	CreationTime time.Time
	Permissions  UserPermission
	Disabled     bool
}

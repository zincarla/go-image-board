package interfaces

//UserPermission Permissions are based on a 64bit uint64, the following is comparisons for that
type UserPermission uint64

//So quick overview of how this will work
//We choose powers of 2
//1, 2, 4, 8... because in binary these map to specific bits
//0001, 0010, 0100, 1000
//When you bit-and two numbers together, only bits that are both 1 will be kept.
//Since each permission only has 1 bit with 1, we can do a bitand and see if the result is the same as the permission we are look for
//Example, permission 3 is 0011, we want to see if this permission has AddTags, which is 2 or 0010.
//0010 & 0011 = 0010, so the output is same as the permission we are checking, user has that permission
//.... Why did I type this, this is not my first time using this method to check permissions...
const (
	//ViewImagesAndTags View only access to Images and Tags
	ViewImagesAndTags UserPermission = 0
	//ModifyImageTags Allows a user to add and remove tags to/from an image, but not create or delete tags themselves
	ModifyImageTags UserPermission = 1
	//AddTags Allows a user to add a new tag to the system (But not delete)
	AddTags UserPermission = 2
	//ModifyTags Allows a user to modify a tag from the system
	ModifyTags UserPermission = 4
	//RemoveTags Allows a user to remove a tag from the system
	RemoveTags UserPermission = 8
	//UploadImage Allows a user to upload an image
	UploadImage UserPermission = 16 //Note that it is probably a good idea to ensure users have ModifyImageTags at a minimum
	//RemoveOtherImages Allows a user to remove an uploaded image. (Note that we can short circuit this in other code to allow a user to remove their own images)
	RemoveImage UserPermission = 32
	//DisableUser Allows a user to disable another user
	DisableUser UserPermission = 64
	//EditUserPermissions Allows a user to edit permissions of another user
	EditUserPermissions UserPermission = 128
	//BulkTagOperations
	BulkTagOperations UserPermission = 256
	//ScoreImage Allows a user to vote on an image's score
	ScoreImage UserPermission = 512
	//SourceImage Allows a user to change an image's source
	SourceImage UserPermission = 1024
	//AddCollections Allows a user to add a new collection to the system (But not delete)
	AddCollections UserPermission = 2048
	//ModifyCollections Allows a user to modify a collection in the system
	ModifyCollections UserPermission = 4096
	//RemoveCollections Allows a user to remove a collection from the system
	RemoveCollections UserPermission = 8192
	//ModifyCollectionMembers Allows a user to add and remove images to/from a collection, but not create or delete collections themselves
	ModifyCollectionMembers UserPermission = 16384
	//Add more permissions here as needed in future. Keep using powers of 2 for this to work.
	//Max number will be 18446744073709551615, after 64 possible permission assignments.
)

//HasPermission checks the current permission set to see if it matches the provided permission
func (Permission UserPermission) HasPermission(CheckPermission UserPermission) bool {
	return (Permission & CheckPermission) == CheckPermission
}

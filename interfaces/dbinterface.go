package interfaces

//DBInterface is a generic interface to allow swappable databases
type DBInterface interface {
	////Account operations
	//CreateUser is used to create and add a user to the AuthN database (return nil on success)
	CreateUser(userName string, password []byte, email string, permissions uint64) error
	//ValidateUser Validate a user's password (return nil if valid)
	ValidateUser(userName string, password []byte) error
	//SetUserPassword Update a user's password, validation of user provided by either old password, or security answers. (nil on success)
	SetUserPassword(userName string, password []byte, newPassword []byte, answerOne []byte, answerTwo []byte, answerThree []byte) error
	//ValidateToken Validate a cookie token (true if valid cookie, false otherwise, error for reason or nil)
	ValidateToken(userName string, tokenID string, ip string) error
	//GenerateToken Generate a cookie token (string token, or error)
	GenerateToken(userName string, ip string) (string, error)
	//RevokeToken Revokes a token (nil on success)
	RevokeToken(userName string) error
	//RemoveUser Removes a user from the AuthN database (nil on success)
	RemoveUser(userName string) error
	//SetSecurityQuestions changes a user's security questions (nil if success)
	SetSecurityQuestions(userName string, questionOne string, questionTwo string, questionThree string, answerOne []byte, answerTwo []byte, answerThree []byte, challengeAnswer []byte) error
	//ValidateSecurityQuestions Validates answers against a user's security questions (nil on success)
	ValidateSecurityQuestions(userName string, answerOne []byte, answerTwo []byte, answerThree []byte) error
	//GetSecurityQuestions returns the three questions, first, second, third, and an error if an issue occured
	GetSecurityQuestions(userName string) (string, string, string, error)
	//GetUserPermissionSet returns a UserPermission object representing a user's intended access
	GetUserPermissionSet(userName string) (UserPermission, error)
	//SetUserPermissionSet sets a user's permission in the database
	SetUserPermissionSet(userID uint64, permissions uint64) error
	//SetUserDisableState disables or enables a user account
	SetUserDisableState(userID uint64, isDisabled bool) error
	//ValidatePasswordStrength Returns an error if there is an issue with the password describing the issue. Else nil
	ValidatePasswordStrength(password string) error
	//GetUserID returns a user's DBID for association with other db elements
	GetUserID(userName string) (uint64, error)
	//GetImage returns an ImageInformation object given an ID
	GetImage(ID uint64) (ImageInformation, error)
	//ValidateProposedUsername returns whether a username is in a valid format
	ValidateProposedUsername(UserName string) error
	//SetImageRating changes a given image's rating
	SetImageRating(ID uint64, Rating string) error
	//SetImageSource changes a given image's source
	SetImageSource(ID uint64, Source string) error
	//GetUserFilter returns the raw string of the user's filter
	GetUserFilter(UserID uint64) (string, error)
	//SearchUsers performs a search for users (Returns a list of UserInfos, or error)
	SearchUsers(searchString string, PageStart uint64, PageStride uint64) ([]UserInformation, uint64, error)

	//Image operations
	//NewImage adds an image with the provided information and returns the id, or error
	NewImage(ImageName string, ImageFileName string, OwnerID uint64, Source string) (uint64, error)
	//DeleteImage removes an image from the db
	DeleteImage(ImageID uint64) error
	//SearchImages performs a search for images (Returns a list of imageIDs, or error)
	SearchImages(Tags []TagInformation, PageStart uint64, PageStride uint64) ([]ImageInformation, uint64, error)
	//GetPrevNexImages performs a search for images (Returns a list of ImageInformations (Up to 2) and an error/nil)
	GetPrevNexImages(Tags []TagInformation, TargetID uint64) ([]ImageInformation, error)

	//GetQueryTags returns a slice of tags based on a query
	GetQueryTags(UserQuery string, CollectionContext bool) ([]TagInformation, error)
	//GetUserFilterTags returns a slice of tags based on a user's custom filter
	GetUserFilterTags(UserID uint64, CollectionContext bool) ([]TagInformation, error)
	//SetUserQueryTags sets a user's global filter
	SetUserQueryTags(UserID uint64, Filter string) error
	//GetImageTags returns a list of TagInformation for all tags that apply to the given image
	GetImageTags(ImageID uint64) ([]TagInformation, error)
	//GetAllTags returns a list of all tags
	GetAllTags() ([]TagInformation, error)
	//GetTag return detailed information on one tag
	GetTag(ID uint64) (TagInformation, error)
	//Tag Operations
	//NewTag adds a tag with the provided information
	NewTag(Name string, Description string, UploaderID uint64) (uint64, error)
	//DeleteTag removes a tag
	DeleteTag(TagID uint64) error
	//AddTag adds an association of a tag to image into the association table
	AddTag(TagID uint64, ImageID uint64, LinkerID uint64) error
	//RemoveTag remove a tag association
	RemoveTag(TagID uint64, ImageID uint64) error
	//UpdateTag updates a pre-existing tag
	UpdateTag(TagID uint64, Name string, Description string, AliasedID uint64, IsAlias bool, UploadID uint64) error
	//BulkAddTag Adds tags to images that already have another tag
	BulkAddTag(TagID uint64, OldTagID uint64, LinkerID uint64) error
	//ReplaceImageTags Replaces an old tag, with the new tag
	ReplaceImageTags(OldTagID uint64, NewTagID uint64, LinkerID uint64) error
	//SearchTags returns a list of tags like the provided name, but only the ID, Name, Description, and IsAlias
	SearchTags(name string, PageStart uint64, PageStride uint64) ([]TagInformation, uint64, error)

	//UpdateUserVoteScore Either creates or changes a user's vote on an image
	UpdateUserVoteScore(UserID uint64, ImageID uint64, Score int64) error
	//UpdateScoreOnImage update ScoreTotal, ScoreAverage, and ScoreVoters on an image
	UpdateScoreOnImage(ImageID uint64) error
	//GetUserVoteScore Returns a user's vote on an image
	GetUserVoteScore(UserID uint64, ImageID uint64) (int64, error)

	//Maitenance
	//InitDatabase connects to a database, and if needed, creates and or updates tables
	InitDatabase() error
	//GetPluginInformation Return plugin info as string
	GetPluginInformation() string
	//AddAuditLog adds a new audit log to the db
	AddAuditLog(UserID uint64, Type string, Info string) error

	//Collections
	//NewCollection adds a collection with the provided information, returns collection ID and/or error
	NewCollection(Name string, Description string, UploaderID uint64) (uint64, error)
	//UpdateCollection changes a basic property of a collection
	UpdateCollection(CollectionID uint64, Name string, Description string) error
	//AddCollectionMember adds an image to a collection
	AddCollectionMember(CollectionID uint64, ImageID uint64, LinkerID uint64) error
	//UpdateCollectionMember updates an image's properties in a collection
	UpdateCollectionMember(CollectionID uint64, ImageID uint64, Order uint64) error
	//RemoveCollectionMember removes an image from collection
	RemoveCollectionMember(CollectionID uint64, ImageID uint64) error
	//DeleteCollection removes a collection
	DeleteCollection(CollectionID uint64) error
	//GetCollections returns a list of Collections
	GetCollections(PageStart uint64, PageStride uint64) ([]CollectionInformation, uint64, error)
	//GetTag return detailed information on one tag
	GetCollection(ID uint64) (CollectionInformation, error)
	//GetCollectionByName returns detailed information on one collection
	GetCollectionByName(Name string) (CollectionInformation, error)
	//GetCollectionMembers gets a list of images in a collection (Returns a list of imageIDs, the count of the total members, and or error)
	GetCollectionMembers(CollectionID uint64, PageStart uint64, PageStride uint64) ([]ImageInformation, uint64, error)
	//GetCollectionsWithImage returns a slice of collections with a specific image
	GetCollectionsWithImage(ImageID uint64) ([]CollectionInformation, error)
	//SearchCollections performs a search for collections (Returns a list of CollectionInformation a result count and an error/nil)
	SearchCollections(Tags []TagInformation, PageStart uint64, PageStride uint64) ([]CollectionInformation, uint64, error)
	//GetCollectionTags returns a list of TagInformation for all tags that apply to the given collection
	GetCollectionTags(CollectionID uint64) ([]TagInformation, error)
}

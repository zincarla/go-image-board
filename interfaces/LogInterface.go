package interfaces

//LogPlugin Provides generic pattern for log plugins
type LogPlugin interface {
	//Add log [LogSource] - [Category] - [User] - [Result] - [Details]
	WriteLog(logSource string, category string, user string, result string, details []string)
	//GetVersionInformation should return "Version - Additional Metadata"
	GetVersionInformation() string
}
package interfaces

//LogPlugin Provides generic pattern for log plugins
type LogPlugin interface {
	//Adds a log entry to the backend log implementation
	WriteLog(logLevel int64, logSource string, user string, result string, details []string)
	//GetVersionInformation should return "Version - Additional Metadata"
	GetVersionInformation() string
	//Init prepares the logging plugin
	Init(targetLogLevel int64, whiteList string, blackList string)
}

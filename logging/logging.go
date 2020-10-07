package logging

import (
	"go-image-board/interfaces"
)

//LogInterface represent access to the back-end logger
var LogInterface interfaces.LogPlugin

//WriteLog writes logs to the back end logger with filtering based on config settings
func WriteLog(logLevel int64, logSource string, user string, result string, details []string) {
	LogInterface.WriteLog(logLevel, logSource, user, result, details)
}

//Is the log referencing a success or failure?
const (
	//ResultSuccess represents a log due to a success
	ResultSuccess = "SUCCESS"
	//ResultFailure represents a log due to a failure
	ResultFailure = "FAIL"
	//ResultInfo represents a log for general information
	ResultInfo = "INFO"
)

//How important is the log?
const (
	//LogLevelCritical represents a critical log event
	LogLevelCritical = int64(0)
	//LogLevelError represents an error log
	LogLevelError = int64(10)
	//LogLevelWarning represents a warning log
	LogLevelWarning = int64(20)
	//LogLevelInfo represents a log for info
	LogLevelInfo = int64(30)
	//LogLevelDebug represents a log for debug
	LogLevelDebug = int64(40)
	//LogLevelVerbose represents a verbose log
	LogLevelVerbose = int64(50)
)

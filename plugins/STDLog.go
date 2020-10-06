package plugins

import (
	"log"
	"strconv"
	"time"
)

//STDLog provides a struct for the logging interface, this will log data to the output console in a format similiar to [item] - [item] - [item]
type STDLog struct {
}

//WriteLog writes the requested log entry to console
func (SLog STDLog) WriteLog(logLevel int64, logSource string, user string, result string, details []string) {
	fullLine := time.Now().Format(time.UnixDate) + " - " + strconv.FormatInt(logLevel, 10) + " - " + logSource + " - " + user + " - " + result + " - "
	for _, detail := range details {
		fullLine = fullLine + detail + "; "
	}
	fullLine = fullLine[:len(fullLine)-2]
	log.Print(fullLine)
}

//GetVersionInformation returns the version and name of this plugin
func (SLog STDLog) GetVersionInformation() string {
	return "STDLog Version 1.0.1.1"
}

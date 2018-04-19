package plugins

import (
	"log"
)

//STDLog provides a struct for the logging interface, this will log data to the output console in a format similiar to [item] - [item] - [item]
type STDLog struct {
}

//WriteLog writes the requested log entry to console
func (SLog STDLog) WriteLog(logSource string, category string, user string, result string, details []string) {
	fullLine := logSource + " - " + category + " - " + user + " - " + result + " - "
	for _, detail := range details {
		fullLine = fullLine + detail + " - "
	}
	fullLine = fullLine[:len(fullLine)-3]
	log.Print(fullLine)
}

//GetVersionInformation returns the version and name of this plugin
func (SLog STDLog) GetVersionInformation() string {
	return "STDLog Version 1.0.0.0"
}

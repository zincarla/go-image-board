package plugins

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//STDLog provides a struct for the logging interface, this will log data to the output console in a format similiar to [item] - [item] - [item]
type STDLog struct {
	targetLogLevel int64
	whiteListRegex *regexp.Regexp
	blackListRegex *regexp.Regexp
}

//WriteLog writes the requested log entry to console
func (SLog *STDLog) WriteLog(logLevel int64, logSource string, user string, result string, details []string) {
	if logLevel <= SLog.targetLogLevel {
		fullLine := time.Now().Format(time.UnixDate) + " - " + strconv.FormatInt(logLevel, 10) + " - " + logSource + " - " + user + " - " + result + " - "
		for _, detail := range details {
			fullLine = fullLine + detail + "; "
		}
		fullLine = fullLine[:len(fullLine)-2]
		//Check if passes regex
		if SLog.whiteListRegex != nil && !SLog.whiteListRegex.MatchString(fullLine) {
			return
		}
		if SLog.blackListRegex != nil && SLog.blackListRegex.MatchString(fullLine) {
			return
		}
		log.Print(fullLine)
	}
}

//Init prepares the logging plugin
func (SLog *STDLog) Init(targetLogLevel int64, whiteList string, blackList string) {
	SLog.targetLogLevel = 100
	SLog.whiteListRegex = nil
	SLog.blackListRegex = nil
	if strings.TrimSpace(whiteList) != "" {
		whiteListRegex, err := regexp.Compile(whiteList)
		if err != nil {
			SLog.WriteLog(0, "STDLog/Init", "", "ERROR", []string{"Failed to compile regex whitelist", whiteList})
		}
		SLog.whiteListRegex = whiteListRegex
	}
	if strings.TrimSpace(blackList) != "" {
		blackListRegex, err := regexp.Compile(blackList)
		if err != nil {
			SLog.WriteLog(0, "STDLog/Init", "", "ERROR", []string{"Failed to compile regex blacklist", blackList})
		}
		SLog.blackListRegex = blackListRegex
	}
	SLog.targetLogLevel = targetLogLevel
}

//GetVersionInformation returns the version and name of this plugin
func (SLog STDLog) GetVersionInformation() string {
	return "STDLog Version 1.0.1.2"
}

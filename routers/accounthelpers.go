package routers

import (
	"errors"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/sessions"
)

//getSessionInformation returns userName, tokenID, and the session itself if the user token is valid, if it is not, it returns the userName, "", and the session. If the session is not valid it returns "","",session.
func getSessionInformation(request *http.Request) (string, string, *sessions.Session) {
	//Get Session
	session, err := config.SessionStore.Get(request, config.SessionVariableName)
	if err != nil {
		//Note that this just gobbles the error. Functions that call this should redirect when tokenID is "" or userName is ""
		//If the user is supposed to be logged in that is
		logging.WriteLog(logging.LogLevelError, "accountrouter/getSessionInformation", "0", logging.ResultFailure, []string{err.Error()})
		return "", "", session
	}
	// Get some session values.
	userName, _ := session.Values["UserName"].(string)
	userName = strings.ToLower(userName)
	tokenID, _ := session.Values["TokenID"].(string)
	ip, _, _ := net.SplitHostPort(request.RemoteAddr)
	if err := database.DBInterface.ValidateToken(userName, tokenID, ip); err != nil {
		return "", "", session
	}
	return userName, tokenID, session
}

//ValidateProposedEmail is a helper function to determine wether an e-mail is valid or not.
func ValidateProposedEmail(email string) error {
	match, err := regexp.MatchString("^\\w+([-+.']\\w+)*@\\w+([-.]\\w+)*\\.\\w+([-.]\\w+)*$", email)
	if match == false {
		return errors.New("invalid e-mail... or at least a really odd one")
	}
	if err != nil {
		return err
	}
	return nil
}

//prepareAnswer trims and simplifies an answer to a security question to ensure that users are more likely to match the answer even if capitalization is off.
func prepareAnswer(answer string) string {
	//Regex to match all whitespace
	regexWhiteSpace := regexp.MustCompile("\\s")
	//Set answers to lower, then remove whitespace
	return regexWhiteSpace.ReplaceAllString(strings.ToLower(answer), "")
}

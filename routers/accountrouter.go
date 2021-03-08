package routers

import (
	"errors"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/sessions"
)

//LogonRouter handles requests to /logon
func LogonRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)

	//If logged in
	if TemplateInput.UserInformation.ID != 0 {
		//Get question for challenge
		oldQOne, _, _, err := database.DBInterface.GetSecurityQuestions(TemplateInput.UserInformation.Name)
		if err == nil && oldQOne != "" {
			TemplateInput.QuestionOne = oldQOne
		}
	}

	//Grab user query information
	switch command := strings.ToLower(request.FormValue("command")); command {
	case "validate":
		username := strings.ToLower(request.FormValue("userName"))
		ip, _, _ := net.SplitHostPort(request.RemoteAddr)
		if username != "" && request.FormValue("password") != "" {
			err := database.DBInterface.ValidateUser(username, []byte(request.FormValue("password")))
			if err == nil {
				//Get Session
				_, _, session := getSessionInformation(request)

				// Set some session values.
				Token, err := database.DBInterface.GenerateToken(username, ip)
				if err != nil {
					logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", username, logging.ResultFailure, []string{"Account Validation", err.Error()})
					TemplateInput.HTMLMessage += template.HTML("Token Failure.<br>")
					replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
					return
				}
				session.Values["TokenID"] = Token
				session.Values["UserName"] = username
				// Save it before we write to the response/return from the handler.
				session.Save(request, responseWriter)
				go WriteAuditLogByName(username, "LOGON", username+" successfully logged on.")
				logging.WriteLog(logging.LogLevelInfo, "accountrouter/LogonRouter", username, logging.ResultSuccess, []string{"Account Validation"})
				redirectWithFlash(responseWriter, request, "/images", "Logged on successfully", "LogonSuccess")
				return
			}
			go WriteAuditLogByName(username, "LOGON", username+" failed to log in. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Wrong username or password.<br>")
			replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
			return
		}
		logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Validation", "Either username, password, or e-mail was left blank, or was not set correctly."})
		TemplateInput.HTMLMessage += template.HTML("Either username, password, or e-mail was left blank, or was not set correctly.<br>")
		replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
		return
	case "create":
		if config.Configuration.AllowAccountCreation == false {
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Creation", "Not allowed by configuration option."})

			TemplateInput.HTMLMessage = template.HTML("Create failed, creations not allowed on this server. (Private?)<br>")
			replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
			return
		}
		if request.FormValue("password") == "" || request.FormValue("password") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Creation", "Password is not correct/confirmed"})
			TemplateInput.HTMLMessage += template.HTML("Create failed, your password is either blank, or the passwords do not match.<br>")
			replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
			return
		}
		username := strings.ToLower(request.FormValue("userName"))
		if username == "" || database.DBInterface.ValidateProposedUsername(username) != nil {
			//If username is blank
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", username, logging.ResultFailure, []string{"Account Creation", "Username failed, either blank or invalid"})
			TemplateInput.HTMLMessage += template.HTML("Create failed, your username is blank or invalid.<br>")
			replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
			return
		}
		if request.FormValue("eMail") == "" || validateProposedEmail(strings.ToLower(request.FormValue("eMail"))) != nil {
			//If username is blank
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", username, logging.ResultFailure, []string{"Account Creation", "E-Mail is either blank, or not formatted correctly"})
			TemplateInput.HTMLMessage += template.HTML("Create failed, your E-Mail is incorrectly formatted.<br>")
			replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
			return
		}
		err := database.DBInterface.CreateUser(username, []byte(request.FormValue("password")), strings.ToLower(request.FormValue("eMail")), config.Configuration.DefaultPermissions)
		if err == nil {
			go WriteAuditLogByName(username, "ACCOUNT-CREATED", username+" successfully created an account.")
			TemplateInput.HTMLMessage += template.HTML("Your account has been created. Please sign in.<br>")
			TemplateInput.UserInformation.ID = 0
			TemplateInput.UserInformation.Name = ""
			break //Break out of switch and reply with template
		}
		logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", username, logging.ResultFailure, []string{"Account Creation", err.Error()})

		TemplateInput.HTMLMessage += template.HTML("Account creation failed.<br>")
	case "logout":
		//User requests logout manually, destroy session
		userName, tokenID, session := getSessionInformation(request)
		//Wipe local session information
		session.Values["TokenID"] = ""
		session.Values["UserName"] = ""
		session.Save(request, responseWriter)

		//But only wipe token from DB if session info was correct
		//The idea is to prevent a DOS in the event someone constructs an invalid session
		if tokenID != "" {
			err := database.DBInterface.RevokeToken(userName)
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Account logout requested but error occured during token removal", err.Error()})
			}
		}

		go WriteAuditLogByName(userName, "LOGOUT", userName+" manually logged out.")
		TemplateInput.HTMLMessage += template.HTML("Successfully logged out.<br>")
		TemplateInput.UserInformation.ID = 0
		TemplateInput.UserInformation.Name = ""
		TemplateInput.QuestionOne = ""
	case "resetpassword":
		//User requests to reset password

		//Verify username is not blank
		userName, _, _ := getSessionInformation(request)
		if userName == "" {
			userName = strings.ToLower(request.FormValue("userName"))
		}
		if userName == "" {
			TemplateInput.HTMLMessage += template.HTML("Username must be specified, please try again.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Username was not set during password reset request"})
			break
		}

		//Cleanup answers (Set to lower, remove white-space)

		answerOne := prepareAnswer(request.FormValue("qonea"))
		answerTwo := prepareAnswer(request.FormValue("qtwoa"))
		answerThree := prepareAnswer(request.FormValue("qthreea"))

		//If all answers are blank, then reply with security questions
		if answerOne == "" && answerTwo == "" && answerThree == "" {
			questionOne, questionTwo, questionThree, err := database.DBInterface.GetSecurityQuestions(userName)
			if err != nil {
				TemplateInput.HTMLMessage += template.HTML("Could not load security questions. Please contact site owner.<br>")
				logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Security Questions not found"})
				break
			}
			TemplateInput.QuestionOne = questionOne
			TemplateInput.QuestionTwo = questionTwo
			TemplateInput.QuestionThree = questionThree
			TemplateInput.UserInformation.Name = userName
			logging.WriteLog(logging.LogLevelVerbose, "accountrouter/LogonRouter", userName, logging.ResultInfo, []string{"Security Questions prompted"})
			break
		}

		//Verify all answers are not blank
		if answerOne == "" || answerTwo == "" || answerThree == "" {
			TemplateInput.HTMLMessage += template.HTML("All 3 questions and answers must be set.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Security Answers not set as user did not fill out form"})
			break
		}

		//Validate new password
		if request.FormValue("newpassword") == "" || request.FormValue("newpassword") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Account resetpassword", "Password is not confirmed"})
			TemplateInput.HTMLMessage += template.HTML("Reset password failed, your new password is either blank, or the confirmation password does not match.<br>")
			break
		}

		//Validate questions/answers in DB
		//Call verify in DB

		err := database.DBInterface.ValidateSecurityQuestions(userName, []byte(answerOne), []byte(answerTwo), []byte(answerThree))
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to validate answers.<br>")
			go WriteAuditLogByName(userName, "PASSWORD-RESET", userName+" failed to reset password, security answers incorrect.")
			break
		}

		//Call change pw

		err = database.DBInterface.SetUserPassword(userName, nil, []byte(request.FormValue("newpassword")), []byte(answerOne), []byte(answerTwo), []byte(answerThree))
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to change password.<br>")
			go WriteAuditLogByName(userName, "PASSWORD-RESET", userName+" failed to reset password. "+err.Error())
			break
		}
		go WriteAuditLogByName(userName, "PASSWORD-RESET", userName+" reset password successfully by security question challenge.")
		TemplateInput.HTMLMessage += template.HTML("Successfully set password.<br>")
	case "changepw":
		//User requests to change password, first validate parameters
		username := strings.ToLower(request.FormValue("userName"))
		//Validate new password
		if request.FormValue("newpassword") == "" || request.FormValue("newpassword") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", username, logging.ResultFailure, []string{"Account ChangePW", "Password is not correct or not confirmed"})
			TemplateInput.HTMLMessage += template.HTML("Change password failed, your new password is either blank, or the passwords do not match.<br>")
			break
		}
		//Validate user is who they are by running validation like logging in
		if username != "" && request.FormValue("oldpassword") != "" && database.DBInterface.ValidateProposedUsername(username) == nil {
			err := database.DBInterface.ValidateUser(username, []byte(request.FormValue("oldpassword")))
			if err != nil {
				go WriteAuditLogByName(username, "PASSWORD-SET", username+" failed to set password. "+err.Error())
				TemplateInput.HTMLMessage += template.HTML("Either username or password incorrect.<br>")
				break
			}
		}

		err := database.DBInterface.SetUserPassword(username, []byte(request.FormValue("oldpassword")), []byte(request.FormValue("newpassword")), nil, nil, nil)
		if err != nil {
			go WriteAuditLogByName(username, "PASSWORD-SET", username+" failed to set password. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Failed to update password.<br>")
			break
		}
		//Success if we hit this point
		//Clear session info to force login
		err = database.DBInterface.RevokeToken(username)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", username, logging.ResultFailure, []string{"Account logout after password change attempted but error occured during token removal", err.Error()})
		}

		_, _, session := getSessionInformation(request)
		session.Values["TokenID"] = ""
		session.Values["UserName"] = ""
		session.Save(request, responseWriter)
		go WriteAuditLogByName(username, "PASSWORD-SET", username+" successfully set password by old password challenge. ")
		TemplateInput.HTMLMessage += template.HTML("Your password was changed successfully. Please log in.<br>")
	case "securityquestion":
		//User requests to change security questions

		userName, tokenID, _ := getSessionInformation(request)
		if tokenID == "" || userName == "" {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform this action.<br>")
			go WriteAuditLogByName(userName, "QUESTION-SET", userName+" failed to set questions. Not logged in.")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"User not logged in"})
			break
		}
		TemplateInput.UserInformation.Name = userName
		//Verify password confirmation
		err := database.DBInterface.ValidateUser(userName, []byte(request.FormValue("confirmpassword")))
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Password confirmation failed, please try again.<br>")
			go WriteAuditLogByName(userName, "QUESTION-SET", userName+" failed to set questions. "+err.Error())
			break
		}

		//Cleanup answers (Set to lower, remove white-space)
		answerOne := prepareAnswer(request.FormValue("qonea"))
		answerTwo := prepareAnswer(request.FormValue("qtwoa"))
		answerThree := prepareAnswer(request.FormValue("qthreea"))
		answerChallenge := prepareAnswer(request.FormValue("qoneca"))
		//No processing on questions
		questionOne := request.FormValue("qone")
		questionTwo := request.FormValue("qtwo")
		questionThree := request.FormValue("qthree")

		//Verify questions are set as well as passwords
		if answerOne == "" || answerTwo == "" || answerThree == "" || questionOne == "" || questionTwo == "" || questionThree == "" {
			TemplateInput.HTMLMessage += template.HTML("All 3 questions and answers must be set.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Security Question not set as user did not fill out form"})
			break
		}

		//Verify Challenge
		oldQOne, _, _, err := database.DBInterface.GetSecurityQuestions(userName)
		if err == nil && oldQOne != "" && answerChallenge == "" {
			TemplateInput.HTMLMessage += template.HTML("You must answer the challenge question.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Security Question not set as user did not answer challenge"})
			break
		}

		//Set questions in db
		//Call change in DB once implemented
		err = database.DBInterface.SetSecurityQuestions(userName, questionOne, questionTwo, questionThree, []byte(answerOne), []byte(answerTwo), []byte(answerThree), []byte(answerChallenge))
		if err != nil {
			go WriteAuditLogByName(userName, "QUESTION-SET", userName+" failed to set questions. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Failed to set questions.<br>")
		} else {
			go WriteAuditLogByName(userName, "QUESTION-SET", userName+" successfully set questions with password challenge. ")
			TemplateInput.HTMLMessage += template.HTML("Successfully set questions.<br>")
		}
	case "setuserfilter":
		//Ensure signed in
		userName, tokenID, _ := getSessionInformation(request)
		if tokenID == "" || userName == "" {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform this action.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"User not logged in"})
			break
		}
		err := database.DBInterface.SetUserQueryTags(TemplateInput.UserInformation.ID, request.FormValue("filter"))
		if err != nil {
			go WriteAuditLogByName(userName, "FILTER-SET", userName+" failed to set filter. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Failed to update filter.<br>")
			break
		}
		//Success
		go WriteAuditLogByName(userName, "FILTER-SET", userName+" successfully set filter. ")
		TemplateInput.HTMLMessage += template.HTML("Your filter was changed successfully.<br>")
	}

	//Populate user filter if signed in
	userName, tokenID, _ := getSessionInformation(request)
	if tokenID != "" && userName != "" {
		TemplateInput.UserFilter, _ = database.DBInterface.GetUserFilter(TemplateInput.UserInformation.ID)
	}
	replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
}

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

//validateProposedEmail is a helper function to determine wether an e-mail is valid or not.
func validateProposedEmail(email string) error {
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

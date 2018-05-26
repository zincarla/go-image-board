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

//LogonRouter handles requests to /logon
func LogonRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getNewTemplateInput(request)

	//If logged in
	if TemplateInput.UserID != 0 {
		//Get question for challenge
		oldQOne, _, _, err := database.DBInterface.GetSecurityQuestions(TemplateInput.UserName)
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
					logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", username, "ERROR", []string{"Account Validation", err.Error()})
					TemplateInput.Message = "Token Failure"
					replyWithTemplate("logon.html", TemplateInput, responseWriter)
					return
				}
				session.Values["TokenID"] = Token
				session.Values["UserName"] = username
				// Save it before we write to the response/return from the handler.
				session.Save(request, responseWriter)
				go writeAuditLogByName(username, "LOGON", username+" successfully logged on.")
				logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", username, "SUCCESS", []string{"Account Validation"})
				http.Redirect(responseWriter, request, "/images", 302)
				return
			}
			go writeAuditLogByName(username, "LOGON", username+" failed to log in. "+err.Error())
			TemplateInput.Message = "Wrong username or password"
			replyWithTemplate("logon.html", TemplateInput, responseWriter)
			return
		}
		logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", "*", "ERROR", []string{"Account Validation", "Either username, password, or e-mail was left blank, or was not set correctly."})
		TemplateInput.Message = "Either username, password, or e-mail was left blank, or was not set correctly."
		replyWithTemplate("logon.html", TemplateInput, responseWriter)
		return
	case "create":
		if config.Configuration.AllowAccountCreation == false {
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", "*", "ERROR", []string{"Account Creation", "Not allowed by configuration option."})

			TemplateInput.Message = "Create failed, creations not allowed on this server. (Private?)"
			replyWithTemplate("logon.html", TemplateInput, responseWriter)
			return
		}
		if request.FormValue("password") == "" || request.FormValue("password") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", "*", "ERROR", []string{"Account Creation", "Password is not correct/confirmed"})
			TemplateInput.Message = "Create failed, your password is either blank, or the passwords do not match"
			replyWithTemplate("logon.html", TemplateInput, responseWriter)
			return
		}
		username := strings.ToLower(request.FormValue("userName"))
		if username == "" || database.DBInterface.ValidateProposedUsername(username) != nil {
			//If username is blank
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", username, "ERROR", []string{"Account Creation", "Username failed, either blank or invalid"})
			TemplateInput.Message = "Create failed, your username is blank or invalid"
			replyWithTemplate("logon.html", TemplateInput, responseWriter)
			return
		}
		if request.FormValue("eMail") == "" || validateProposedEmail(strings.ToLower(request.FormValue("eMail"))) != nil {
			//If username is blank
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", username, "ERROR", []string{"Account Creation", "E-Mail is either blank, or not formatted correctly"})
			TemplateInput.Message = "Create failed, your E-Mail is incorrectly formatted"
			replyWithTemplate("logon.html", TemplateInput, responseWriter)
			return
		}
		err := database.DBInterface.CreateUser(username, []byte(request.FormValue("password")), strings.ToLower(request.FormValue("eMail")), config.Configuration.DefaultPermissions)
		if err == nil {
			go writeAuditLogByName(username, "ACCOUNT-CREATED", username+" successfully created an account.")
			TemplateInput.Message += "Your account has been created. Please sign in. "
			TemplateInput.UserID = 0
			TemplateInput.UserName = ""
			break //Break out of switch and reply with template
		}
		logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", username, "ERROR", []string{"Account Creation", err.Error()})

		TemplateInput.Message = "Account creation failed " // + err.Error()
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
				logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", userName, "WARN", []string{"Account logout requested but error occured during token removal", err.Error()})
			}
		}

		go writeAuditLogByName(userName, "LOGOUT", userName+" manually logged out.")
		TemplateInput.Message = "Successfully logged out."
		TemplateInput.UserID = 0
		TemplateInput.UserName = ""
		TemplateInput.QuestionOne = ""
	case "resetpassword":
		//User requests to reset password

		//Verify username is not blank
		userName, _, _ := getSessionInformation(request)
		if userName == "" {
			userName = strings.ToLower(request.FormValue("userName"))
		}
		if userName == "" {
			TemplateInput.Message = "Username must be specified, please try again."
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", userName, "WARN", []string{"Username was not set during password reset request"})
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
				TemplateInput.Message = "Could not load security questions. Please contact site owner."
				logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", userName, "ERROR", []string{"Security Questions not found"})
				break
			}
			TemplateInput.QuestionOne = questionOne
			TemplateInput.QuestionTwo = questionTwo
			TemplateInput.QuestionThree = questionThree
			TemplateInput.UserName = userName
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", userName, "INFO", []string{"Security Questions prompted"})
			break
		}

		//Verify all answers are not blank
		if answerOne == "" || answerTwo == "" || answerThree == "" {
			TemplateInput.Message = "All 3 questions and answers must be set."
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", userName, "ERROR", []string{"Security Answers not set as user did not fill out form"})
			break
		}

		//Validate new password
		if request.FormValue("newpassword") == "" || request.FormValue("newpassword") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", userName, "ERROR", []string{"Account resetpassword", "Password is not confirmed"})
			TemplateInput.Message = "Reset password failed, your new password is either blank, or the confirmation password does not match"
			break
		}

		//Validate questions/answers in DB
		//Call verify in DB

		err := database.DBInterface.ValidateSecurityQuestions(userName, []byte(answerOne), []byte(answerTwo), []byte(answerThree))
		if err != nil {
			TemplateInput.Message = "Failed to validate answers"
			go writeAuditLogByName(userName, "PASSWORD-RESET", userName+" failed to reset password, security answers incorrect.")
			break
		}

		//Call change pw

		err = database.DBInterface.SetUserPassword(userName, nil, []byte(request.FormValue("newpassword")), []byte(answerOne), []byte(answerTwo), []byte(answerThree))
		if err != nil {
			TemplateInput.Message = "Failed to change password"
			go writeAuditLogByName(userName, "PASSWORD-RESET", userName+" failed to reset password. "+err.Error())
			break
		}
		go writeAuditLogByName(userName, "PASSWORD-RESET", userName+" reset password successfully by security question challenge.")
		TemplateInput.Message = "Successfully set password."
	case "changepw":
		//User requests to change password, first validate parameters
		username := strings.ToLower(request.FormValue("userName"))
		//Validate new password
		if request.FormValue("newpassword") == "" || request.FormValue("newpassword") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.LogInterface.WriteLog("AccountRouter", "AccountHandler", username, "ERROR", []string{"Account ChangePW", "Password is not correct or not confirmed"})
			TemplateInput.Message = "Change password failed, your new password is either blank, or the passwords do not match"
			break
		}
		//Validate user is who they are by running validation like logging in
		if username != "" && request.FormValue("oldpassword") != "" && database.DBInterface.ValidateProposedUsername(username) == nil {
			err := database.DBInterface.ValidateUser(username, []byte(request.FormValue("oldpassword")))
			if err != nil {
				go writeAuditLogByName(username, "PASSWORD-SET", username+" failed to set password. "+err.Error())
				TemplateInput.Message = "Either username or password incorrect."
				break
			}
		}

		err := database.DBInterface.SetUserPassword(username, []byte(request.FormValue("oldpassword")), []byte(request.FormValue("newpassword")), nil, nil, nil)
		if err != nil {
			go writeAuditLogByName(username, "PASSWORD-SET", username+" failed to set password. "+err.Error())
			TemplateInput.Message = "Failed to update password."
			break
		}
		//Success if we hit this point
		//Clear session info to force login
		err = database.DBInterface.RevokeToken(username)
		if err != nil {
			logging.LogInterface.WriteLog("LogonRouter", "LogonHandler", username, "WARN", []string{"Account logout after password change attempted but error occured during token removal", err.Error()})
		}

		_, _, session := getSessionInformation(request)
		session.Values["TokenID"] = ""
		session.Values["UserName"] = ""
		session.Save(request, responseWriter)
		go writeAuditLogByName(username, "PASSWORD-SET", username+" successfully set password by old password challenge. ")
		TemplateInput.Message = "Your password was changed successfully. Please log in."
	case "securityquestion":
		//User requests to change security questions

		userName, tokenID, _ := getSessionInformation(request)
		if tokenID == "" || userName == "" {
			TemplateInput.Message = "You must be logged in to perform this action."
			go writeAuditLogByName(userName, "QUESTION-SET", userName+" failed to set questions. Not logged in.")
			logging.LogInterface.WriteLog("AccountRouter", "AccountHandler", userName, "ERROR", []string{"User not logged in"})
			break
		}
		TemplateInput.UserName = userName
		//Verify password confirmation
		err := database.DBInterface.ValidateUser(userName, []byte(request.FormValue("confirmpassword")))
		if err != nil {
			TemplateInput.Message = "Password confirmation failed, please try again."
			go writeAuditLogByName(userName, "QUESTION-SET", userName+" failed to set questions. "+err.Error())
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
			TemplateInput.Message = "All 3 questions and answers must be set."
			logging.LogInterface.WriteLog("AccountRouter", "AccountHandler", userName, "ERROR", []string{"Security Question not set as user did not fill out form"})
			break
		}

		//Verify Challenge
		oldQOne, _, _, err := database.DBInterface.GetSecurityQuestions(userName)
		if err == nil && oldQOne != "" && answerChallenge == "" {
			TemplateInput.Message = "You must answer the challenge question"
			logging.LogInterface.WriteLog("AccountRouter", "AccountHandler", userName, "ERROR", []string{"Security Question not set as user did not answer challenge"})
			break
		}

		//Set questions in db
		//Call change in DB once implemented
		err = database.DBInterface.SetSecurityQuestions(userName, questionOne, questionTwo, questionThree, []byte(answerOne), []byte(answerTwo), []byte(answerThree), []byte(answerChallenge))
		if err != nil {
			go writeAuditLogByName(userName, "QUESTION-SET", userName+" failed to set questions. "+err.Error())
			TemplateInput.Message = "Failed to set questions."
		} else {
			go writeAuditLogByName(userName, "QUESTION-SET", userName+" successfully set questions with password challenge. ")
			TemplateInput.Message = "Successfully set questions."
		}
	}

	replyWithTemplate("logon.html", TemplateInput, responseWriter)
}

//getSessionInformation returns userName, tokenID, and the session itself if the user token is valid, if it is not, it returns the userName, "", and the session. If the session is not valid it returns "","",session.
func getSessionInformation(request *http.Request) (string, string, *sessions.Session) {
	//Get Session
	session, err := config.SessionStore.Get(request, config.SessionVariableName)
	if err != nil {
		//Note that this just gobbles the error. Functions that call this should redirect when tokenID is "" or userName is ""
		//If the user is supposed to be logged in that is
		logging.LogInterface.WriteLog("LogonRouter", "getSessionInformation", "*", "WARN", []string{err.Error()})
		return "", "", session
	}
	// Get some session values.
	userName, _ := session.Values["UserName"].(string)
	userName = strings.ToLower(userName)
	tokenID, _ := session.Values["TokenID"].(string)
	ip, _, _ := net.SplitHostPort(request.RemoteAddr)
	if err := database.DBInterface.ValidateToken(userName, tokenID, ip); err != nil {
		return userName, "", session
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

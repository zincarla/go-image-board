package routers

import (
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"html/template"
	"net"
	"net/http"
	"strings"
)

//LogonGetRouter handles get requests to /logon
func LogonGetRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)

	//If logged in
	if TemplateInput.IsLoggedOn() {
		//Get question for challenge
		oldQOne, _, _, err := database.DBInterface.GetSecurityQuestions(TemplateInput.UserInformation.Name)
		if err == nil && oldQOne != "" {
			TemplateInput.QuestionOne = oldQOne
		}
		//Get user filter
		TemplateInput.UserFilter, _ = database.DBInterface.GetUserFilter(TemplateInput.UserInformation.ID)
	}

	//Grab user query information
	switch command := strings.ToLower(request.FormValue("command")); command {
	case "logout":
		//User requests logout manually, destroy session, I don't care about logout being "GET"'d
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
		if strings.ToLower(request.FormValue("userName")) != "" {
			userName = strings.ToLower(request.FormValue("userName"))
		}
		if userName == "" {
			TemplateInput.HTMLMessage += template.HTML("Username must be specified, please try again.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Username was not set during password reset request"})
			break
		}

		//We are in a "GET" only method, so return questions and let them POST answers
		questionOne, questionTwo, questionThree, err := database.DBInterface.GetSecurityQuestions(userName)
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Could not load security questions. Please contact site owner.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", userName, logging.ResultFailure, []string{"Security Questions not found for user", userName})
			break
		}
		TemplateInput.QuestionOne = questionOne
		TemplateInput.QuestionTwo = questionTwo
		TemplateInput.QuestionThree = questionThree
		//Since we maybe attempting to reset password for the non-logged in user, treat this as non-logged in.
		TemplateInput.UserInformation.Name = userName
		TemplateInput.UserInformation.ID = 0
		logging.WriteLog(logging.LogLevelVerbose, "accountrouter/LogonRouter", userName, logging.ResultInfo, []string{"Security Questions prompted"})
		break
	}

	replyWithTemplate("logon.html", TemplateInput, responseWriter, request)
}

//LogonPostRouter handles post requests to /logon
func LogonPostRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)

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
					logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Validation", err.Error()})
					TemplateInput.HTMLMessage += template.HTML("Token Failure.<br>")
					redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonFailed")
					return
				}
				session.Values["TokenID"] = Token
				session.Values["UserName"] = username
				// Save it before we write to the response/return from the handler.
				session.Save(request, responseWriter)
				go WriteAuditLogByName(username, "LOGON", username+" successfully logged on.")
				logging.WriteLog(logging.LogLevelInfo, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultSuccess, []string{"Account Validation"})
				redirectWithFlash(responseWriter, request, "/images", TemplateInput.HTMLMessage, "LogonSucceeded")
				return
			}
			go WriteAuditLogByName(username, "LOGON", username+" failed to log in. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Wrong username or password.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonFailed")
			return
		}
		logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Validation", "Either username, password, or e-mail was left blank, or was not set correctly."})
		TemplateInput.HTMLMessage += template.HTML("Either username, password, or e-mail was left blank, or was not set correctly.<br>")
		redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonFailed")
		return
	case "create":
		if config.Configuration.AllowAccountCreation == false {
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Creation", "Not allowed by configuration option."})

			TemplateInput.HTMLMessage = template.HTML("Create failed, creations not allowed on this server. (Private?)<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountFailed")
			return
		}
		if request.FormValue("password") == "" || request.FormValue("password") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Creation", "Password is not correct/confirmed"})
			TemplateInput.HTMLMessage += template.HTML("Create failed, your password is either blank, or the passwords do not match.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountFailed")
			return
		}
		username := strings.ToLower(request.FormValue("userName"))
		if username == "" || database.DBInterface.ValidateProposedUsername(username) != nil {
			//If username is blank
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", username, logging.ResultFailure, []string{"Account Creation", "Username failed, either blank or invalid"})
			TemplateInput.HTMLMessage += template.HTML("Create failed, your username is blank or invalid.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountFailed")
			return
		}
		if request.FormValue("eMail") == "" || ValidateProposedEmail(strings.ToLower(request.FormValue("eMail"))) != nil {
			//If username is blank
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Creation", "E-Mail is either blank, or not formatted correctly"})
			TemplateInput.HTMLMessage += template.HTML("Create failed, your E-Mail is incorrectly formatted.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountFailed")
			return
		}
		err := database.DBInterface.CreateUser(username, []byte(request.FormValue("password")), strings.ToLower(request.FormValue("eMail")), config.Configuration.DefaultPermissions)
		if err == nil {
			go WriteAuditLogByName(username, "ACCOUNT-CREATED", username+" successfully created an account.")
			TemplateInput.HTMLMessage += template.HTML("Your account has been created. Please sign in.<br>")
			TemplateInput.UserInformation.ID = 0
			TemplateInput.UserInformation.Name = ""
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountSucceeded")
			return
		}
		logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account Creation", err.Error()})
		TemplateInput.HTMLMessage += template.HTML("Account creation failed.<br>")
		redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountFailed")
		return
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
				logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account logout requested but error occured during token removal", err.Error()})
			}
		}

		go WriteAuditLogByName(userName, "LOGOUT", userName+" manually logged out.")
		TemplateInput.HTMLMessage += template.HTML("Successfully logged out.<br>")
		TemplateInput.UserInformation.ID = 0
		TemplateInput.UserInformation.Name = ""
		TemplateInput.QuestionOne = ""
		redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogoutSucceeded")
		return
	case "resetpassword":
		//User requests to reset password

		//Verify username is not blank
		userName, _, _ := getSessionInformation(request)
		if strings.ToLower(request.FormValue("userName")) != "" {
			userName = strings.ToLower(request.FormValue("userName"))
			TemplateInput.UserInformation.Name = userName
			TemplateInput.UserInformation.ID = 0
		}
		if userName == "" {
			TemplateInput.HTMLMessage += template.HTML("Username must be specified, please try again.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Username was not set during password reset request"})
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
			return
		}

		//Cleanup answers (Set to lower, remove white-space)
		answerOne := prepareAnswer(request.FormValue("qonea"))
		answerTwo := prepareAnswer(request.FormValue("qtwoa"))
		answerThree := prepareAnswer(request.FormValue("qthreea"))

		//Verify all answers are not blank
		if answerOne == "" || answerTwo == "" || answerThree == "" {
			TemplateInput.HTMLMessage += template.HTML("All 3 answers must be set.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Security Answers not set as user did not fill out form"})
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
			return
		}

		//Validate new password
		if request.FormValue("newpassword") == "" || request.FormValue("newpassword") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account resetpassword", "Password is not confirmed"})
			TemplateInput.HTMLMessage += template.HTML("Reset password failed, your new password is either blank, or the confirmation password does not match.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
			return
		}

		//Validate questions/answers in DB
		//Call verify in DB
		err := database.DBInterface.ValidateSecurityQuestions(userName, []byte(answerOne), []byte(answerTwo), []byte(answerThree))
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to validate answers.<br>")
			go WriteAuditLogByName(userName, "PASSWORD-RESET", userName+" failed to reset password, security answers incorrect.")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
			return
		}

		//Call change pw
		err = database.DBInterface.SetUserPassword(userName, nil, []byte(request.FormValue("newpassword")), []byte(answerOne), []byte(answerTwo), []byte(answerThree))
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Failed to change password.<br>")
			go WriteAuditLogByName(userName, "PASSWORD-RESET", userName+" failed to reset password. "+err.Error())
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
			return
		}
		go WriteAuditLogByName(userName, "PASSWORD-RESET", userName+" reset password successfully by security question challenge.")
		TemplateInput.HTMLMessage += template.HTML("Successfully set password.<br>")
		redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordSucceeded")
		return
	case "changepw":
		//User requests to change password, first validate parameters
		username := strings.ToLower(request.FormValue("userName"))
		//Validate new password
		if request.FormValue("newpassword") == "" || request.FormValue("newpassword") != request.FormValue("confirmpassword") {
			//If password is blank or password does not match confirmed password
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account ChangePW", "Password is not correct or not confirmed"})
			TemplateInput.HTMLMessage += template.HTML("Change password failed, your new password is either blank, or the passwords do not match.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
			return
		}
		//Validate user is who they are by running validation like logging in
		if username != "" && request.FormValue("oldpassword") != "" && database.DBInterface.ValidateProposedUsername(username) == nil {
			err := database.DBInterface.ValidateUser(username, []byte(request.FormValue("oldpassword")))
			if err != nil {
				go WriteAuditLogByName(username, "PASSWORD-SET", username+" failed to set password. "+err.Error())
				TemplateInput.HTMLMessage += template.HTML("Either username or password incorrect.<br>")
				redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
				return
			}
		}

		err := database.DBInterface.SetUserPassword(username, []byte(request.FormValue("oldpassword")), []byte(request.FormValue("newpassword")), nil, nil, nil)
		if err != nil {
			go WriteAuditLogByName(username, "PASSWORD-SET", username+" failed to set password. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Failed to update password.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordFailed")
			return
		}
		//Success if we hit this point
		//Clear session info to force login
		err = database.DBInterface.RevokeToken(username)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Account logout after password change attempted but error occured during token removal", err.Error()})
		}

		_, _, session := getSessionInformation(request)
		session.Values["TokenID"] = ""
		session.Values["UserName"] = ""
		session.Save(request, responseWriter)
		go WriteAuditLogByName(username, "PASSWORD-SET", username+" successfully set password by old password challenge. ")
		TemplateInput.HTMLMessage += template.HTML("Your password was changed successfully. Please log in again.<br>")
		redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "PasswordSucceeded")
		return
	case "securityquestion":
		//User requests to change security questions

		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform this action.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "QUESTION-SET", TemplateInput.UserInformation.Name+" failed to set questions. Not logged in.")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"User not logged in"})
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "QuestionFailed")
			return
		}

		//Verify password confirmation
		err := database.DBInterface.ValidateUser(TemplateInput.UserInformation.Name, []byte(request.FormValue("confirmpassword")))
		if err != nil {
			TemplateInput.HTMLMessage += template.HTML("Password confirmation failed, please try again.<br>")
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "QUESTION-SET", TemplateInput.UserInformation.Name+" failed to set questions. "+err.Error())
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "QuestionFailed")
			return
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
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Security Question not set as user did not fill out form"})
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "QuestionFailed")
			return
		}

		//Verify Challenge
		oldQOne, _, _, err := database.DBInterface.GetSecurityQuestions(TemplateInput.UserInformation.Name)
		if err == nil && oldQOne != "" && answerChallenge == "" {
			TemplateInput.HTMLMessage += template.HTML("You must answer the challenge question.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"Security Question not set as user did not answer challenge"})
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "QuestionFailed")
			return
		}

		//Set questions in db
		//Call change in DB once implemented
		err = database.DBInterface.SetSecurityQuestions(TemplateInput.UserInformation.Name, questionOne, questionTwo, questionThree, []byte(answerOne), []byte(answerTwo), []byte(answerThree), []byte(answerChallenge))
		if err != nil {
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "QUESTION-SET", TemplateInput.UserInformation.Name+" failed to set questions. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Failed to set questions.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "QuestionFailed")
			return
		}
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "QUESTION-SET", TemplateInput.UserInformation.Name+" successfully set questions with password challenge. ")
		TemplateInput.HTMLMessage += template.HTML("Successfully set questions.<br>")
		redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "QuestionSucceed")
		return
	case "setuserfilter":
		//Ensure signed in
		if !TemplateInput.IsLoggedOn() {
			TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform this action.<br>")
			logging.WriteLog(logging.LogLevelError, "accountrouter/LogonRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"User not logged in"})
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "LogonRequired")
			return
		}
		err := database.DBInterface.SetUserQueryTags(TemplateInput.UserInformation.ID, request.FormValue("filter"))
		if err != nil {
			go WriteAuditLogByName(TemplateInput.UserInformation.Name, "FILTER-SET", TemplateInput.UserInformation.Name+" failed to set filter. "+err.Error())
			TemplateInput.HTMLMessage += template.HTML("Failed to update filter.<br>")
			redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountFail")
			return
		}
		//Success
		go WriteAuditLogByName(TemplateInput.UserInformation.Name, "FILTER-SET", TemplateInput.UserInformation.Name+" successfully set filter. ")
		TemplateInput.HTMLMessage += template.HTML("Your filter was changed successfully.<br>")
		redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "FilterSucceeded")
		return
	}
	TemplateInput.HTMLMessage += template.HTML("Command not recognized or form submitted incorrectly.<br>")
	redirectWithFlash(responseWriter, request, "/logon", TemplateInput.HTMLMessage, "AccountFail")
	return
}

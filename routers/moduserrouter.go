package routers

import (
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"html/template"
	"net/http"
	"strconv"
)

//ModUserRouter serves requests to /mod/user
func ModUserRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)

	if request.FormValue("userName") == "" {
		TemplateInput.HTMLMessage += template.HTML("You must specify a user to edit.<br>")
	} else {
		//Get Command
		switch cmd := request.FormValue("command"); cmd {
		case "editUserPerms":
			//Check if logged in
			if TemplateInput.UserInformation.ID == 0 {
				TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
				break
			}
			//Check if has permissions
			if TemplateInput.UserPermissions.HasPermission(interfaces.EditUserPermissions) != true {
				TemplateInput.HTMLMessage += template.HTML("User does not have modify permission for user permissions.<br>")

				go WriteAuditLog(TemplateInput.UserInformation.ID, "EDIT-USERPERMISSIONS", TemplateInput.UserInformation.Name+" failed to edit user permissions, insufficient permissions.")
				break
			}
			//Do the thing
			iPerms, err := strconv.ParseUint(request.FormValue("permissions"), 10, 64)
			if err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to parse permission value.<br>")
				break
			}
			sUserName := request.FormValue("userName")
			iUserID, err := database.DBInterface.GetUserID(sUserName)
			if err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to find user.<br>")
				break
			}
			if err := database.DBInterface.SetUserPermissionSet(iUserID, iPerms); err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to update user in database.<br>")
			} else {
				TemplateInput.HTMLMessage += template.HTML("Successfully set the user's permissions.<br>")
			}
		case "disableUser":
			//Check if logged in
			if TemplateInput.UserInformation.ID == 0 {
				TemplateInput.HTMLMessage += template.HTML("You must be logged in to perform that action.<br>")
				break
			}
			//Check if has permissions
			if TemplateInput.UserPermissions.HasPermission(interfaces.DisableUser) != true {
				TemplateInput.HTMLMessage += template.HTML("User does not have disable permission for users.<br>")
				go WriteAuditLog(TemplateInput.UserInformation.ID, "DISABLE-USER", TemplateInput.UserInformation.Name+" failed to edit user permissions, insufficient permissions.")
				break
			}
			//Do the thing
			sDisableState := request.FormValue("isDisabled")
			bDisableState, err := strconv.ParseBool(sDisableState)
			if err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to parse disable state<br>")
				break
			}
			sUserName := request.FormValue("userName")
			iUserID, err := database.DBInterface.GetUserID(sUserName)
			if err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to find user<br>")
				break
			}
			if err := database.DBInterface.SetUserDisableState(iUserID, bDisableState); err != nil {
				TemplateInput.HTMLMessage += template.HTML("Failed to set user disable state in database<br>")
			} else {
				TemplateInput.HTMLMessage += template.HTML("Successfully set the user's disable state.<br>")
			}
		}
		if modUserID, err := database.DBInterface.GetUserID(request.FormValue("userName")); err == nil {
			if TemplateInput.ModUserData, err = database.DBInterface.GetUser(modUserID); err != nil {
				TemplateInput.HTMLMessage += template.HTML("Could not get userdata.<br>")
				logging.WriteLog(logging.LogLevelError, "moduserrouter/ModUserRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"SQL error occured getting userdata ", err.Error()})
			}
		} else {
			TemplateInput.HTMLMessage += template.HTML("Could not get userdata, do they exist?<br>")
		}
	}

	redirectWithFlash(responseWriter, request, "/mod", TemplateInput.HTMLMessage, "ModUser")
}

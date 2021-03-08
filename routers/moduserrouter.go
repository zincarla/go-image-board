package routers

import (
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"net/http"
	"strconv"
)

//ModUserRouter serves requests to /mod/user
func ModUserRouter(responseWriter http.ResponseWriter, request *http.Request) {
	TemplateInput := getTemplateInputFromRequest(responseWriter, request)

	if request.FormValue("userName") == "" {
		TemplateInput.Message += "You must specify a user to edit. "
	} else {
		//Get Command
		switch cmd := request.FormValue("command"); cmd {
		case "editUserPerms":
			//Check if logged in
			if TemplateInput.UserInformation.ID == 0 {
				TemplateInput.Message += "You must be logged in to perform that action. "
				break
			}
			//Check if has permissions
			if TemplateInput.UserPermissions.HasPermission(interfaces.EditUserPermissions) != true {
				TemplateInput.Message += "User does not have modify permission for user permissions. "

				go WriteAuditLog(TemplateInput.UserInformation.ID, "EDIT-USERPERMISSIONS", TemplateInput.UserInformation.Name+" failed to edit user permissions, insufficient permissions.")
				break
			}
			//Do the thing
			iPerms, err := strconv.ParseUint(request.FormValue("permissions"), 10, 64)
			if err != nil {
				TemplateInput.Message += "Failed to parse permission value"
				break
			}
			sUserName := request.FormValue("userName")
			iUserID, err := database.DBInterface.GetUserID(sUserName)
			if err != nil {
				TemplateInput.Message += "Failed to find user"
				break
			}
			if err := database.DBInterface.SetUserPermissionSet(iUserID, iPerms); err != nil {
				TemplateInput.Message += "Failed to update user in database"
			} else {
				TemplateInput.Message += "Successfully set " + sUserName + "'s permissions."
			}
		case "disableUser":
			//Check if logged in
			if TemplateInput.UserInformation.ID == 0 {
				TemplateInput.Message += "You must be logged in to perform that action. "
				break
			}
			//Check if has permissions
			if TemplateInput.UserPermissions.HasPermission(interfaces.DisableUser) != true {
				TemplateInput.Message += "User does not have disable permission for users. "
				go WriteAuditLog(TemplateInput.UserInformation.ID, "DISABLE-USER", TemplateInput.UserInformation.Name+" failed to edit user permissions, insufficient permissions.")
				break
			}
			//Do the thing
			sDisableState := request.FormValue("isDisabled")
			bDisableState, err := strconv.ParseBool(sDisableState)
			if err != nil {
				TemplateInput.Message += "Failed to parse disable state"
				break
			}
			sUserName := request.FormValue("userName")
			iUserID, err := database.DBInterface.GetUserID(sUserName)
			if err != nil {
				TemplateInput.Message += "Failed to find user"
				break
			}
			if err := database.DBInterface.SetUserDisableState(iUserID, bDisableState); err != nil {
				TemplateInput.Message += "Failed to set user disable state in database"
			} else {
				TemplateInput.Message += "Successfully set " + sUserName + "'s disable state."
			}
		}
		if modUserID, err := database.DBInterface.GetUserID(request.FormValue("userName")); err == nil {
			if TemplateInput.ModUserData, err = database.DBInterface.GetUser(modUserID); err != nil {
				TemplateInput.Message += "Could not get userdata. "
				logging.WriteLog(logging.LogLevelError, "moduserrouter/ModUserRouter", TemplateInput.UserInformation.GetCompositeID(), logging.ResultFailure, []string{"SQL error occured getting userdata ", err.Error()})
			}
		} else {
			TemplateInput.Message += "Could not get userdata, do they exist? "
		}
	}

	redirectWithFlash(responseWriter, request, "/mod", TemplateInput.Message, "ModUser")
}

package routers

import (
	"go-image-board/config"
	"net/http"
)

//AccountRequiredMiddleWare ensures a user is logged in and redirects it not
func AccountRequiredMiddleWare(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if config.Configuration.AccountRequiredToView {
			TemplateInput, ok := request.Context().Value(TemplateInputKeyID).(templateInput)
			if (!ok || !TemplateInput.IsLoggedOn()) && config.Configuration.AccountRequiredToView {
				redirectWithFlash(responseWriter, request, "/logon", "Access to this server requires an account", "LogonRequired")
				return
			}
		}
		next(responseWriter, request) // call next on the original handler
	})
}

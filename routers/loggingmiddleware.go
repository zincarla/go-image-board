package routers

import (
	"context"
	"go-image-board/logging"
	"net/http"
)

//ContextKeyID should be used when setting values in a context to prevent collisions
type ContextKeyID string

//TemplateInputKeyID is the context key for TemplateInput objects.
const TemplateInputKeyID = ContextKeyID("TemplateInput")

//LogMiddleware provides verbose logging on all requests, and initializes the TemplateInput
func LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		TemplateInput := getNewTemplateInput(responseWriter, request)
		logging.WriteLog(logging.LogLevelVerbose, "loggingmiddleware/LogMiddleware", TemplateInput.UserInformation.GetCompositeID(), logging.ResultInfo, []string{request.RequestURI})
		//Save template input to context
		request = request.WithContext(context.WithValue(request.Context(), TemplateInputKeyID, TemplateInput))
		next.ServeHTTP(responseWriter, request) // call ServeHTTP on the original handler
	})
}

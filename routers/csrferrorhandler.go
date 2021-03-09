package routers

import (
	"go-image-board/logging"
	"net/http"

	"github.com/gorilla/csrf"
)

//CSRFErrorRouter handles CSRF related errors, it logs and reports to requestor.
func CSRFErrorRouter(res http.ResponseWriter, req *http.Request) {
	logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{csrf.FailureReason(req).Error()})
	//Check for cookie
	userMessage := "You did not pass the CSRF check. "
	cookie, err := req.Cookie("_gorilla_csrf")
	if err != nil {
		logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to check csrf cookie", err.Error()})
		userMessage += "This is probably because you/your browser did not send the CSRF cookie. "
	} else {
		logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"CSRF cookie found with value of ", cookie.Value})
	}
	logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Header token is", req.Header.Get("X-CSRF-Token")})
	if req.Header.Get("X-CSRF-Token") == "" {
		userMessage += "This is probably because you/your browser did not send the X-CSRF-Token header. "
	}
	http.Error(res, userMessage, http.StatusBadRequest)
}

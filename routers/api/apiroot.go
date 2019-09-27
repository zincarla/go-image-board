package api

import (
	"encoding/json"
	"go-image-board/logging"
	"net/http"
)

//ReplyWithJSON replies to a request with the specified interface to be marshaled to a JOSN object
func ReplyWithJSON(responseWriter http.ResponseWriter, request *http.Request, jsonObject interface{}, userName string) {
	response, err := json.Marshal(jsonObject)
	if err != nil {
		logging.LogInterface.WriteLog("APIRouter", "ReplyWithJSON", userName, "ERROR", []string{"Error during reply", err.Error()})
		http.Error(responseWriter, "", http.StatusInternalServerError)
		return
	}
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.Write(response)
}

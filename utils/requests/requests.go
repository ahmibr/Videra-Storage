package requests

import (
	"fmt"
	"net/http"

	"github.com/SayedAlesawy/Videra-Storage/utils/errors"
)

// TimeStampLayout Layout for timestamp fields
var TimeStampLayout = "2006-01-02T15:04:05.000Z"

// HandleRequestError A function to handle http request failure
func HandleRequestError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	w.Write([]byte(message))
}

// ValidateUploadHeaders is a function to check existance of parameters inside header
func ValidateUploadHeaders(h *http.Header, params ...string) error {
	for _, param := range params {
		if h.Get(param) == "" {
			return errors.New(fmt.Sprintf("%s header not provided", param))
		}
	}

	return nil
}

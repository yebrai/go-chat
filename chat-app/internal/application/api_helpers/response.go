package api_helpers

import (
	"encoding/json"
	"log"
	"net/http"
)

// RespondWithJSON sends a JSON response with the given status code and payload.
func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON response: %v", err)
		// Fallback to a generic error if marshalling fails
		http.Error(w, `{"error":"Failed to marshal JSON response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err = w.Write(response)
	if err != nil {
		log.Printf("Error writing JSON response: %v", err)
	}
}

// RespondWithError sends a JSON error response with the given status code and message.
// The error message is wrapped in a consistent JSON structure: {"error": "message"}.
func RespondWithError(w http.ResponseWriter, code int, message string) {
	errorPayload := map[string]string{"error": message}
	RespondWithJSON(w, code, errorPayload)
}

// ErrorResponse is a generic structure for error messages.
type ErrorResponse struct {
	Error string `json:"error"`
}

// DecodeJSONBody attempts to decode the request body into the provided value.
// It returns an error if decoding fails or if the body is empty.
func DecodeJSONBody(r *http.Request, dst interface{}) error {
	if r.Body == nil {
		return errors.New("request body is empty")
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Optional: be strict about request fields

	err := decoder.Decode(dst)
	if err != nil {
		// Handle various types of errors, e.g., EOF for empty body, syntax errors
		return err
	}
	return nil
}

// Placeholder for errors package, should be imported if not already
var errorsNew = func(text string) error { return &customError{text} }
type customError struct{ s string }
func (e *customError) Error() string { return e.s }

func NewError(text string) error {
    return errorsNew(text)
}

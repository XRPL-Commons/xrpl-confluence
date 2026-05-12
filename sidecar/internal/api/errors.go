package api

// Error codes (closed set). Add new codes here when new endpoints introduce them.
const (
	ErrCodeScenarioInvalid      = "scenario_invalid"
	ErrCodeScenarioUnreadable   = "scenario_unreadable"
	ErrCodeFindingNotFound      = "finding_not_found"
	ErrCodeLogsNotFound         = "logs_not_found"
	ErrCodeBadRequest           = "bad_request"
	ErrCodeMethodNotAllowed     = "method_not_allowed"
	ErrCodeUnsupportedMediaType = "unsupported_media_type"
)

// Error is a single, machine-readable error returned by the API or CLI.
// Code is from a closed, documented set per endpoint. Field and Hint are optional.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// ErrorResponse is the envelope for any non-2xx HTTP response.
type ErrorResponse struct {
	Error Error `json:"error"`
}

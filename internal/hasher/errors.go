package hasher

import "fmt"

// Error codes for the hasher package
const (
	ErrCodeInvalidNetwork = 1
	ErrCodeInvalidInput = 2
	ErrCodeNetworkSerialization = 3
	ErrCodeNetworkDeserialization = 4
	ErrCodeInferenceFailure = 5
	ErrCodeNoValidPasses = 6
	ErrCodeConsensusFailure = 7
	ErrCodeValidationFailure = 8
)

// HasherError is a structured error type for the hasher package
type HasherError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *HasherError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("hasher: [%d] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("hasher: [%d] %s", e.Code, e.Message)
}

func NewError(code int, message string, details ...string) error {
	err := &HasherError{
		Code:    code,
		Message: message,
	}
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// Predefined errors
var (
	ErrInvalidNetwork = NewError(ErrCodeInvalidNetwork, "invalid network configuration")
	ErrInvalidInput = NewError(ErrCodeInvalidInput, "invalid input data")
	ErrNetworkSerialization = NewError(ErrCodeNetworkSerialization, "network serialization failed")
	ErrNetworkDeserialization = NewError(ErrCodeNetworkDeserialization, "network deserialization failed")
	ErrInferenceFailure = NewError(ErrCodeInferenceFailure, "inference failed")
	ErrNoValidPasses = NewError(ErrCodeNoValidPasses, "no valid passes completed")
	ErrConsensusFailure = NewError(ErrCodeConsensusFailure, "consensus failed")
	ErrValidationFailure = NewError(ErrCodeValidationFailure, "validation failed")
)

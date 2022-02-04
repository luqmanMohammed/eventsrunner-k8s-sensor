package common

import "fmt"

// RequiredFieldMissingError custom error is returned when required field is missing.
// Missing field is present as part of the error struct
type RequiredFieldMissingError struct {
	Field string
}

// Error function implements error interface
func (rf *RequiredFieldMissingError) Error() string {
	return fmt.Sprintf("required field %s is missing", rf.Field)
}

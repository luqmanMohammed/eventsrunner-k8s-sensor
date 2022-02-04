package common

import "fmt"

// RequiredFieldMissingError custom error is returned when required field is missing.
// Missing field is present as part of the error struct
type RequiredConfigMissingError struct {
	ConfigName string
}

// Error function implements error interface
func (rf *RequiredConfigMissingError) Error() string {
	return fmt.Sprintf("required config %s is missing", rf.ConfigName)
}

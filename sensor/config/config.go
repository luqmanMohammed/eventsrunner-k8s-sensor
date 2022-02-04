package config

import (
	"fmt"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
)

// RequiredFieldMissingError custom error is returned when required field is missing.
// Missing field is present as part of the error struct
type RequiredConfigMissingError struct {
	ConfigName string
}

// Error function implements error interface
func (rf *RequiredConfigMissingError) Error() string {
	return fmt.Sprintf("required config %s is missing", rf.ConfigName)
}

// AnyRequestedConfigMissing is an helper function which will check if any of the
// configs provided in the config map are missing.
// If the value is the zero of the type then its considered as missing.
func AnyRequestedConfigMissing(configs map[string]interface{}) error {
	missingConfig := utils.FindZeroValue(configs)
	if missingConfig != "" {
		return &RequiredConfigMissingError{ConfigName: missingConfig}
	}
	return nil
}

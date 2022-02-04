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

func AnyRequestedConfigMissing(configs map[string]interface{}) error {
	missingConfig := utils.FindZeroValue(configs)
	if missingConfig == "" {
		return &RequiredConfigMissingError{missingConfig}
	}
	return nil
}

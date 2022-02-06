package config

import (
	"fmt"
	"os"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
)

var (
	DefaultConfig = map[string]interface{}{
		// Sensor Config
		"sensorName":     "er-k8s-sensor",
		"kubeConfigPath": "",
		// Rule Collector Config
		"sensorNamespace":          "eventsrunner",
		"sensorRuleConfigMapLabel": "er-k8s-sensor-rules=true",
		// Event Queue
		"workerCount":  10,
		"maxTryCount":  5,
		"requeueDelay": 30 * time.Second,
		// Executor
		"executorType": "log",
		// Script Executor
		"scriptDir":    "",
		"scriptPrefix": "",
		// Events Runner Executor
		"authType":            "",
		"eventsRunnerBaseURL": "",
		"requestTimeout":      0,
		"caCertPath":          "",
		// JWT ER Executor
		"jwtToken": "",
		// mTLS ER Executor
		"clientCertPath": "",
		"clientKeyPath":  "",
	}
)

type Config struct {
	// Sensor Config
	LogVerbosity   int
	SensorName     string
	KubeConfigPath string
	// Rule Collector Config
	SensorNamespace          string
	SensorRuleConfigMapLabel string
	// Event Queue
	WorkerCount  int
	MaxTryCount  int
	RequeueDelay time.Duration
	// Executor
	ExecutorType string
	// Script Executor
	ScriptDir    string
	ScriptPrefix string
	// Events Runner Executor
	AuthType            string
	EventsRunnerBaseURL string
	RequestTimeout      time.Duration
	CaCertPath          string
	// JWT ER Executor
	JWTToken string
	// mTLS ER Executor
	ClientCertPath string
	ClientKeyPath  string
}

// ParseConfigFromViper will collect configs using viper and unmarshal them into
// a Config Struct.
// Viper is configured to collect configs in the following order:
// 1. Default variables
// 2. Environment variables
// 3. Config file
// Environment variables should start with the prefix ER_K8S_SENSOR_ to be collected
// Config file should be in yaml format
// Config files will be collected in the following locations in the following order
// unless if config file path is not provided part of parameters
// 1. /etc/er-k8s-sensor/config.yaml
// 2. $HOME/.er-k8s-sensor/config.yaml
func ParseConfigFromViper(cfgPath string, verbosity int) (*Config, error) {
	for key, value := range DefaultConfig {
		viper.SetDefault(key, value)
	}
	if cfgPath != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgPath)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		if err != nil {
			klog.V(1).ErrorS(err, "failed to get user home directory. File based config wont be taken")
		}
		viper.AddConfigPath("/etc/er-k8s-sensor")
		viper.AddConfigPath(home + "/.er-k8s-sensor")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}
	viper.AllowEmptyEnv(false)
	viper.SetEnvPrefix("ER_K8S_SENSOR")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		klog.V(1).Info("Using config file: ", viper.ConfigFileUsed())
	} else {
		klog.V(1).ErrorS(err, "failed to read config file. skipping config collection from files")
	}
	if verbosity != 0 {
		viper.Set("logVerbosity", verbosity)
	}
	var config *Config
	err := viper.Unmarshal(&config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

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

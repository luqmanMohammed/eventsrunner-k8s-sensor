package utils

import (
	"reflect"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

// GetKubeAPIConfig returns a Kubernetes API config.
// Common abstraction to get config in both incluster and out cluster
// scenarios.
func GetKubeAPIConfig(kubeConfigPath string) (*rest.Config, error) {
	if kubeConfigPath == "" {
		klog.V(3).Info("Provided KubeConfig path is empty. Getting config from home")
		if home := homedir.HomeDir(); home != "" {
			kubeConfigPath = home + "/.kube/config"
		}
	}
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return clientcmd.BuildConfigFromFlags("", "")
	}
	return clientConfig, nil
}

// GetKubeAPIConfigOrDie wrapper around GetKubeAPIConfig.
// Panics if unable to load config.
func GetKubeAPIConfigOrDie(kubeConfigPath string) *rest.Config {
	config, err := GetKubeAPIConfig(kubeConfigPath)
	if err != nil {
		panic(err)
	}
	return config
}

// StringInSlice returns true if the string is in the slice.
func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// IsZero checks if the provided value is its zero value
func IsZero(value interface{}) bool {
	return value == nil || reflect.DeepEqual(value, reflect.Zero(reflect.TypeOf(value)).Interface())
}

// FindZeroValue finds the 1st zero value in the map.
func FindZeroValue(values map[string]interface{}) string {
	for k, v := range values {
		if IsZero(v) {
			return k
		}
	}
	return ""
}

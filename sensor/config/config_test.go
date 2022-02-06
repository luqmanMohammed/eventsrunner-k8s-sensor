package config

import (
	"os"
	"testing"
)

var config string = `---
sensorNamespace: test
`

func TestConfigCollectionOrderAndParsing(t *testing.T) {
	os.WriteFile("/tmp/config.yaml", []byte(config), 0644)
	defer os.Remove("/tmp/config.yaml")
	os.Setenv("ER_K8S_SENSOR_EXECUTORTYPE", "script")
	os.Setenv("ER_K8S_SENSOR_NAMESPACE", "test-env")
	configObj, err := ParseConfigFromViper("/tmp/config.yaml", 1)
	if err != nil {
		t.Fatalf("Error parsing config: %v", err)
	}
	if configObj.SensorName != "er-k8s-sensor" {
		t.Fatalf("Expected sensorName to be er-k8s-sensor, got %s", configObj.SensorName)
	}
	if configObj.SensorNamespace != "test" {
		t.Fatalf("Expected sensorNamespace to be test, got %s", configObj.SensorNamespace)
	}
	if configObj.ExecutorType != "script" {
		t.Fatalf("Expected executorType to be script, got %s", configObj.ExecutorType)
	}
	if configObj.LogVerbosity != 1 {
		t.Fatalf("Expected logVerbosity to be 1, got %d", configObj.LogVerbosity)
	}
}

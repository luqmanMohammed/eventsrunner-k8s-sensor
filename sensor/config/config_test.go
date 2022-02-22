package config

import (
	"os"
	"testing"
)

var config string = `---
sensorNamespace: test-env
executorType: script
`

var homeConfig string = `---
sensorNamespace: home-test
workerCount: 5
`

func TestConfigFileCollectionFromDefaultLocations(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal("Failed to get user home directory")
	}
	if err := os.Mkdir(home+"/.er-k8s-sensor", 0755); err != nil {
		t.Fatalf("Failed to create sensor config dir %v", err)
	}
	defer os.RemoveAll(home + "/.er-k8s-sensor")
	if err := os.WriteFile(home+"/.er-k8s-sensor/config.yaml", []byte(homeConfig), 0644); err != nil {
		t.Fatalf("Failed to write sensor config file %v", err)
	}
	configTest, err := ParseConfigFromViper("", 1)
	if err != nil {
		t.Fatalf("Failed to parse config %v", err)
	}
	if configTest.SensorNamespace != "home-test" {
		t.Fatalf("Expected sensorNamespace to be home-test got %v", configTest.SensorNamespace)
	}
	if configTest.WorkerCount != 5 {
		t.Fatalf("Expected workerCount to be 5 got %v", configTest.WorkerCount)
	}
}

func TestConfigCollectionOrderAndParsing(t *testing.T) {
	os.WriteFile("/tmp/config.yaml", []byte(config), 0644)
	defer os.Remove("/tmp/config.yaml")
	
	os.Setenv("ER_K8S_SENSORNAMESPACE", "test")
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

func TestAnyRequestedConfigMissingFunc(t *testing.T) {
	testMap := map[string]interface{}{
		"test1": 1,
		"test2": "test2",
	}
	if err := AnyRequestedConfigMissing(testMap); err != nil {
		t.Fatalf("Error checking for missing config: %v", err)
	}

	testMap2 := map[string]interface{}{
		"test1": 0,
		"test2": "came",
	}
	if err := AnyRequestedConfigMissing(testMap2); err == nil {
		t.Fatalf("Expected error checking for missing config")
	} else {
		requiredCMerr := err.(*RequiredConfigMissingError)
		if requiredCMerr.ConfigName != "test1" {
			t.Fatalf("Expected test1 missing, got %s", requiredCMerr.ConfigName)
		}
	}
}

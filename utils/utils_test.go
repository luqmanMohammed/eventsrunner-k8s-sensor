package utils

import "testing"

func TestGetKubeAPIConfig(t *testing.T) {
	//Test if config is correctly taken from default location
	//Assumes that the test environment has kubeConfig at $HOME/.kube/config
	if config, err := GetKubeAPIConfig(""); err != nil {
		t.Error("Expected to get cluster config from default location")
	} else if config == nil {
		t.Error("Expected to get cluster config from default location")
	}
}

func TestStringInSlice(t *testing.T) {
	if !StringInSlice("a", []string{"a", "b"}) {
		t.Error("Expected to find string in slice")
	}
	if StringInSlice("x", []string{"a", "b", "a"}) {
		t.Error("Expected to not find string in slice")
	}
}

func TestZeroValueFunction(t *testing.T) {
	testStr := ""
	testInt := 0

	if !IsZero(testStr) {
		t.Error("Expected to be zero value true")
	}
	if !IsZero(testInt) {
		t.Error("Expected to be zero value true")
	}
}

// TestFindZeroValue tests if FindZeroValue returns the correct key
func TestFindZeroValue(t *testing.T) {
	testMap := map[string]interface{}{"a": 1, "b": 2, "c": 3}
	if key := FindZeroValue(testMap); key != "" {
		t.Errorf("Expected to not find zero value in map, got %s", key)
	}
	testMap["a"] = 0
	if key := FindZeroValue(testMap); key != "a" {
		t.Errorf("Expected to find zero value in map, got %s", key)
	}
}

func TestConvertToStringLower(t *testing.T) {
	testStr := "Test"
	if ConvertToStringLower([]string{testStr})[0] != "test" {
		t.Error("Expected to convert string to lowercase")
	}
}

func TestRemoveDuplicateStrings(t *testing.T) {
	testStr := []string{"a", "b", "c", "a"}
	if len(RemoveDuplicateStrings(testStr)) != 3 {
		t.Error("Expected to remove duplicate strings")
	}
}

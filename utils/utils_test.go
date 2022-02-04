package utils

import "testing"

func TestGetKubeAPIConfig(t *testing.T) {
	//Test if config is correctly taken from default location
	//Assumes that the test environment has kubeconfig at $HOME/.kube/config
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
	test_str := ""
	test_int := 0

	if !IsZero(test_str) {
		t.Error("Expected to be zero value true")
	}
	if !IsZero(test_int) {
		t.Error("Expected to be zero value true")
	}
}

// TestFindZeroValue tests if FindZeroValue returns the correct key
func TestFindZeroValue(t *testing.T) {
	test_map := map[string]interface{}{"a": 1, "b": 2, "c": 3}
	if key := FindZeroValue(test_map); key != "" {
		t.Errorf("Expected to not find zero value in map, got %s", key)
	}
	test_map["a"] = 0
	if key := FindZeroValue(test_map); key != "a" {
		t.Errorf("Expected to find zero value in map, got %s", key)
	}
}

package common

import "testing"

func TestGetKubeAPIConfig(t *testing.T) {
	//Test if the function is failing if incluster config is trigger from outside
	if _, err := GetKubeAPIConfig(false, ""); err == nil {
		t.Error("Expected error when running outside cluster")
	}
	//Test if config is correctly taken from default location
	//Assumes that the test environment has kubeconfig at $HOME/.kube/config
	if config, err := GetKubeAPIConfig(true, ""); err != nil {
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

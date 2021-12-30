package common

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

func GetKubeAPIConfig(isLocal bool, kubeConfigPath string) (*rest.Config, error) {
	if isLocal {
		klog.V(3).Info("Client detected to be running in local")
		if kubeConfigPath == "" {
			klog.V(3).Info("Provided KubeConfig path is empty. Getting config from home")
			if home := homedir.HomeDir(); home != "" {
				kubeConfigPath = home + "/.kube/config"
			}
		}
		return clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	} else {
		klog.V(3).Info("Initilizing incluster config")
		return rest.InClusterConfig()
	}
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

package sensor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luqmanMohammed/er-k8s-sensor/common"
	"github.com/luqmanMohammed/er-k8s-sensor/sensor/rules"

	v1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientapiv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	rules_basic = []*rules.Rule{
		{
			Group:      "",
			APIVersion: "v1",
			Resource:   "pods",
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
			Namespaces: []string{"default"},
		},
	}
	rules_reload = []*rules.Rule{
		{
			Group:      "",
			APIVersion: "v1",
			Resource:   "pods",
			EventTypes: []rules.EventType{rules.ADDED},
		},
		{
			Group:      "",
			APIVersion: "v1",
			Resource:   "configmaps",
			Namespaces: []string{"kube-system"},
			EventTypes: []rules.EventType{rules.ADDED},
		},
	}
	rules_custom = []*rules.Rule{
		{
			Group:      "k8ser.io",
			APIVersion: "v1",
			Resource:   "er-crds",
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED},
		},
	}
)

func setupKubconfig() *rest.Config {
	if config, err := common.GetKubeAPIConfig(true, ""); err != nil {
		panic(err)
	} else {
		return config
	}
}

func setupSensor() *Sensor {
	config := setupKubconfig()
	sensor := New(&SensorOpts{
		KubeConfig:  config,
		SensorLabel: "k8s",
	})
	return sensor
}

var (
	errNotFound = errors.New("not found")
	errTimeout  = errors.New("timeout")
)

func checkIfObjectExistsInQueue(retry int, sensor *Sensor, searchObject metav1.Object) error {
	retryCount := 0
	for {
		if sensor.Queue.Len() > 0 {
			item, shutdown := sensor.Queue.Get()
			event := item.(*Event)
			if event.Objects[0].GetName() == searchObject.GetName() &&
				event.Objects[0].GetNamespace() == searchObject.GetNamespace() {
				return nil
			}
			if shutdown {
				return errNotFound
			}
			sensor.Queue.Done(item)
		}
		if retryCount == retry {
			return errTimeout
		} else {
			retryCount++
			time.Sleep(1 * time.Second)
		}
	}
}

func TestSensorStart(t *testing.T) {
	config := setupKubconfig()
	sensor := New(&SensorOpts{
		KubeConfig:  config,
		SensorLabel: "k8s",
	})
	go sensor.Start(rules_basic)
	defer sensor.Stop()
	time.Sleep(3 * time.Second)
	if len(sensor.registeredRules) != 1 {
		t.Error("Failed to start sensor")
	}
}

func TestSensorReload(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rules_basic)
	sensor.ReloadRules(rules_reload)
	if len(sensor.registeredRules) != 2 {
		t.Error("Failed to reload sensor")
	}
	if len(sensor.registeredRules[0].rule.EventTypes) != 1 {
		t.Error("New rules are not correctly loaded")
	}
	if sensor.registeredRules[1].rule.Resource != rules_reload[1].Resource {
		t.Error("New rules not correctly loaded")
	}
	configmap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "kube-system",
		},
	}
	test_configmap, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().ConfigMaps("kube-system").Create(context.Background(), configmap, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Failed to create configmap: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(30, sensor, test_configmap) {
	case errNotFound:
		t.Error("Configmap not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for configmap to be added to queue")
	}
}

func TestEventAdded(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rules_basic)
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test-container",
					Image: "nginx",
				},
			},
		},
	}
	test_pod, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Failed to create pod: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(30, sensor, test_pod) {
	case errNotFound:
		t.Error("Pod not found in queue")
		return
	case errTimeout:
		t.Error("Timeout waiting for pod to be added to queue")
		return
	}
}

func TestCRDCompatibility(t *testing.T) {
	sensor := setupSensor()
	crd := apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "er-crd.k8ser.io",
		},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "k8ser.io",
			Scope: apiextv1.ClusterScoped,
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural:   "er-crds",
				Singular: "er-crd",
				Kind:     "ERCRD",
			},
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
			},
		},
	}
	if _, err := clientapiv1.NewForConfigOrDie(sensor.KubeConfig).CustomResourceDefinitions().Create(context.Background(), &crd, metav1.CreateOptions{}); err != nil {
		t.Errorf("Failed to create CRD: %v", err)
		return
	}
	go sensor.Start(rules_custom)
	
}

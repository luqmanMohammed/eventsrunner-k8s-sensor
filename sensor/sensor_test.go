package sensor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/luqmanMohammed/events-runner-k8s-sensor/common"
	"github.com/luqmanMohammed/events-runner-k8s-sensor/sensor/rules"

	v1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientapiextv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
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
			Resource:   "ercrds",
			Namespaces: []string{"default"},
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED},
		},
	}
	rules_clusterbound = []*rules.Rule{
		{
			Group:      "",
			APIVersion: "v1",
			Resource:   "namespaces",
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
		},
	}
)

func retryFunc(retryFunc func() bool, count int) bool {
	retryCount := 0
	for retryCount < count {
		retryCount++
		if retryFunc() {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

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

func checkIfObjectExistsInQueue(retry int, sensor *Sensor, searchObject metav1.Object, eventType rules.EventType) error {
	retryCount := 0
	for {
		if sensor.Queue.Len() > 0 {
			item, shutdown := sensor.Queue.Get()
			event := item.(*Event)
			if event.Objects[0].GetName() == searchObject.GetName() &&
				event.Objects[0].GetNamespace() == searchObject.GetNamespace() {
				if eventType == rules.NONE {
					return nil
				} else if event.EventType == eventType {
					return nil
				}
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

	if !retryFunc(func() bool {
		fmt.Println(len(sensor.registeredRules))
		if len(sensor.registeredRules) != 1 {
			t.Error("Failed to start sensor")
			return false
		}
		return true
	}, 5) {
		t.Error("Failed to start sensor")
		return
	}
}

func TestSensorReload(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rules_basic)
	sensor.ReloadRules(rules_reload)

	if !retryFunc(func() bool {
		if len(sensor.registeredRules) != 2 {
			t.Error("Failed to reload sensor")
			return false
		}
		if len(sensor.registeredRules[0].rule.EventTypes) != 1 {
			t.Error("New rules are not correctly loaded")
			return false
		}
		if sensor.registeredRules[1].rule.Resource != rules_reload[1].Resource {
			t.Error("New rules not correctly loaded")
			return false
		}
		return true
	}, 5) {
		t.Error("Failed to start sensor")
		return
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
	switch checkIfObjectExistsInQueue(30, sensor, test_configmap, rules.ADDED) {
	case errNotFound:
		t.Error("Configmap not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for configmap to be added to queue")
	}
}

func TestCRDCompatibility(t *testing.T) {
	sensor := setupSensor()
	crd := apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ercrds.k8ser.io",
		},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "k8ser.io",
			Scope: apiextv1.NamespaceScoped,
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural:   "ercrds",
				Singular: "ercrd",
				Kind:     "Ercrd",
			},
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextv1.JSONSchemaProps{
								"spec": {
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := clientapiextv1.NewForConfigOrDie(sensor.KubeConfig).CustomResourceDefinitions().Create(context.Background(), &crd, metav1.CreateOptions{}); err != nil {
		t.Errorf("Failed to create CRD: %v", err)
		return
	}

	go sensor.Start(rules_custom)

	if !retryFunc(func() bool {
		if len(sensor.registeredRules) != 1 {
			t.Logf("Failed to start sensor. Retrying")
			return false
		}
		if sensor.registeredRules[0].rule.Resource != rules_custom[0].Resource {
			t.Logf("Failed to start sensor. Retrying")
			return false
		}
		return true
	}, 5) {
		t.Error("Failed to start sensor")
		return
	}

	crdGVR := schema.GroupVersionResource{
		Group:    "k8ser.io",
		Version:  "v1",
		Resource: "ercrds",
	}

	client := dynamic.NewForConfigOrDie(sensor.KubeConfig)
	crdObj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Ercrd",
			"apiVersion": "k8ser.io/v1",
			"metadata": map[string]interface{}{
				"name":      "test-er-crd",
				"namespace": "default",
			},
			"spec": "test-spec",
		},
	}
	res := client.Resource(crdGVR).Namespace("default")
	crdInst, err := res.Create(context.Background(), &crdObj, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Failed to create CRD instance: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(30, sensor, crdInst, rules.ADDED) {
	case errNotFound:
		t.Error("CRD instance for ADD event not found in queue")
		return
	case errTimeout:
		t.Error("Timeout waiting for CRD instance ADD event to be added to queue")
		return
	}
	crdInst.Object["spec"] = "test-spec-modified"
	updatedCrdInst, err := res.Update(context.Background(), crdInst, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf("Failed to update CRD instance: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(30, sensor, updatedCrdInst, rules.MODIFIED) {
	case errNotFound:
		t.Error("CRD instance for MODIFIED event not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for CRD instance MODIFIED event to be added to queue")
	}
}

func TestClusterBoundResourcesComp(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rules_clusterbound)
	if !retryFunc(func() bool {
		if len(sensor.registeredRules) != 1 {
			t.Logf("Failed to start sensor. Retrying")
			return false
		}
		if sensor.registeredRules[0].rule.Resource != rules_clusterbound[0].Resource {
			t.Logf("Failed to start sensor. Retrying")
			return false
		}
		return true
	}, 5) {
		t.Error("Failed to start sensor")
		return
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}
	nsObj, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Failed to create namespace: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(30, sensor, nsObj, rules.ADDED) {
	case errNotFound:
		t.Error("Namespace not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for Namespace to be added to queue")
	}
}

func TestEventHandlerDynamics(t *testing.T) {

}

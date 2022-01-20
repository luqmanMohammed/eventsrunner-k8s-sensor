package sensor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/common"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"

	appsv1 "k8s.io/api/apps/v1"
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
	rules_basic = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			ID: "test-rule-1",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
			Namespaces: []string{"default"},
		},
	}
	rules_reload = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			ID: "test-rule-1",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			EventTypes: []rules.EventType{rules.ADDED},
		},
		"test-rule-2": {
			ID: "test-rule-2",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "configmaps",
			},
			Namespaces: []string{"kube-system"},
			EventTypes: []rules.EventType{rules.ADDED},
		},
	}
	rules_custom = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			ID: "test-rule-1",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "k8ser.io",
				Version:  "v1",
				Resource: "ercrds",
			},
			Namespaces: []string{"default"},
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED},
		},
	}
	rules_clusterbound = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			ID: "test-rule-1",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
		},
	}
	rules_cache = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			ID: "test-rule-1",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			EventTypes: []rules.EventType{rules.ADDED},
		},
	}
	rules_dynamic = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			EventTypes: []rules.EventType{rules.MODIFIED},
		},
	}
	rules_object_subset = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			EventTypes: []rules.EventType{rules.MODIFIED},
			UpdatesOn:  []rules.K8sObjectSubset{"spec"},
		},
		"test-rule-2": {
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			EventTypes: []rules.EventType{rules.MODIFIED},
			UpdatesOn:  []rules.K8sObjectSubset{"metadata"},
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

func waitStartSensor(t *testing.T, sensor *Sensor, ruleSet map[rules.RuleID]*rules.Rule, waitSecounds int) {
	if !retryFunc(func() bool {
		if len(sensor.ruleInformers) != len(ruleSet) {
			return false
		}
		for ruleID, ruleInformer := range sensor.ruleInformers {
			if ruleInformer.rule.Resource == ruleSet[ruleID].Resource {
				return true
			}
		}
		return false
	}, waitSecounds) {
		t.Error("Failed to start sensor")
		return
	}
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
		KubeConfig:                     config,
		SensorLabel:                    "k8s",
		LoadObjectsDurationBeforeStart: time.Second * 0,
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

func TestObjectsCreatedBeforeSensorStartAreNotAdded(t *testing.T) {
	config := setupKubconfig()
	sensor := New(&SensorOpts{
		KubeConfig:  config,
		SensorLabel: "k8s",
	})
	go sensor.Start(rules_cache)
	defer sensor.Stop()
	waitStartSensor(t, sensor, rules_cache, 10)

	// Create a pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: "kube-system",
		},
	}
	switch checkIfObjectExistsInQueue(5, sensor, pod, rules.ADDED) {
	case nil:
		t.Errorf("Pod should not be added")
	case errTimeout, errNotFound:
		return
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
	if len(sensor.ruleInformers) != 1 {
		t.Error("Failed to start sensor")
	}
}

// TODO: Improve test
func TestSensorReload(t *testing.T) {
	sensor := setupSensor()

	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().ConfigMaps("kube-system").Delete(context.Background(), "test-configmap", metav1.DeleteOptions{})
	}()

	go sensor.Start(rules_basic)
	waitStartSensor(t, sensor, rules_basic, 10)
	sensor.ReloadRules(rules_reload)
	waitStartSensor(t, sensor, rules_reload, 10)
	if len(sensor.ruleInformers) != 2 {
		t.Error("Failed to reload sensor")
	}
	if len(sensor.ruleInformers["test-rule-1"].rule.EventTypes) != 1 {
		t.Error("New rules are not correctly loaded")
	}
	if sensor.ruleInformers["test-rule-2"].rule.Resource != rules_reload["test-rule-2"].Resource {
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
	switch checkIfObjectExistsInQueue(30, sensor, test_configmap, rules.ADDED) {
	case errNotFound:
		t.Error("Configmap not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for configmap to be added to queue")
	}
}

func TestSensorIsWorkingWithCRDs(t *testing.T) {
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

	defer func() {
		clientapiextv1.NewForConfigOrDie(sensor.KubeConfig).CustomResourceDefinitions().Delete(context.Background(), crd.Name, metav1.DeleteOptions{})
	}()

	if _, err := clientapiextv1.NewForConfigOrDie(sensor.KubeConfig).CustomResourceDefinitions().Create(context.Background(), &crd, metav1.CreateOptions{}); err != nil {
		t.Errorf("Failed to create CRD: %v", err)
		return
	}

	time.Sleep(3 * time.Second)

	go sensor.Start(rules_custom)
	waitStartSensor(t, sensor, rules_custom, 10)

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

func TestSensorWorkingWithClusterBoundResources(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rules_clusterbound)
	waitStartSensor(t, sensor, rules_clusterbound, 10)

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
	}()

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

func TestOnlyConfiguredEventListenerIsAdded(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rules_dynamic)
	waitStartSensor(t, sensor, rules_dynamic, 10)

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace-2",
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
	}()

	nsObj, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Failed to create namespace: %v", err)
		return
	}

	switch checkIfObjectExistsInQueue(10, sensor, nsObj, rules.ADDED) {
	case nil:
		t.Errorf("Namespace %s ADDED event should not be added to queue", ns.Name)
		return
	}
	ns.ObjectMeta.Labels = map[string]string{
		"test-label": "test-value",
	}
	nsObj, err = kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Update(context.Background(), ns, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf("Failed to update namespace: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(30, sensor, nsObj, rules.MODIFIED) {
	case errNotFound:
		t.Errorf("Namespace %s MODIFIED event not found in queue", ns.Name)
	case errTimeout:
		t.Errorf("Timeout waiting for Namespace %s MODIFIED event to be added to queue", ns.Name)
	}
}

func TestEnqueueOnlyOnSpecificK8sObjSubsetUpdate(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rules_object_subset)
	waitStartSensor(t, sensor, rules_object_subset, 10)

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace-3",
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
	}()
	_, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Failed to create namespace: %v", err)
		return
	}
	ns.ObjectMeta.Labels = map[string]string{
		"test-label": "test-value",
	}
	nsObj, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Update(context.Background(), ns, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf("Failed to update namespace: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(10, sensor, nsObj, rules.MODIFIED) {
	case nil:
		t.Errorf("Namespace %s Metadata MODIFIED event should not be added to queue", ns.Name)
	}

	replicas := int32(1)
	// Create test deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment-2",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test-label": "test-value",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-2",
					Labels: map[string]string{
						"test-label": "test-value",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "test-container-2",
							Image: "nginx",
						},
					},
				},
			},
		},
	}

	_, err = kubernetes.NewForConfigOrDie(sensor.KubeConfig).AppsV1().Deployments(ns.Name).Create(context.Background(), deployment, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Failed to create deployment: %v", err)
		return
	}

	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).AppsV1().Deployments(ns.Name).Delete(context.Background(), deployment.Name, metav1.DeleteOptions{})
	}()

	switch checkIfObjectExistsInQueue(5, sensor, deployment, rules.ADDED) {
	case nil:
		t.Errorf("Deployment %s ADDED event should not be added to queue", deployment.Name)
	}

	deployment.Spec.Template.Spec.Containers[0].Name = "test-container-2-1"
	deploymentObj, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).AppsV1().Deployments(ns.Name).Update(context.Background(), deployment, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf("Failed to update deployment: %v", err)
		return
	}
	switch checkIfObjectExistsInQueue(5, sensor, deploymentObj, rules.MODIFIED) {
	case errNotFound, errTimeout:
		t.Errorf("Deployment %s MODIFIED event not found in queue", deployment.Name)
	}
}

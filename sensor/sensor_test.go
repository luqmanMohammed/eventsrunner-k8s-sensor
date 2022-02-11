package sensor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/config"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientapiextv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	rulesBasic = map[rules.RuleID]*rules.Rule{
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
	rulesPreReload = map[rules.RuleID]*rules.Rule{
		"test-rule-0": {
			ID: "test-rule-0",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "services",
			},
			EventTypes: []rules.EventType{rules.ADDED},
		},
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
				Resource: "namespaces",
			},
			EventTypes: []rules.EventType{rules.ADDED},
		},
	}
	rulesReload = map[rules.RuleID]*rules.Rule{
		"test-rule-0": {
			ID: "test-rule-0",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "services",
			},
			EventTypes: []rules.EventType{rules.ADDED},
		},
		"test-rule-1": {
			ID: "test-rule-1",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED},
		},
		"test-rule-x": {
			ID: "test-rule-x",
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "configmaps",
			},
			EventTypes: []rules.EventType{rules.ADDED},
			Namespaces: []string{"default"},
		},
	}
	rulesCustom = map[rules.RuleID]*rules.Rule{
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
	rulesClusterbound = map[rules.RuleID]*rules.Rule{
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
	rulesCache = map[rules.RuleID]*rules.Rule{
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
	rulesDynamic = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			EventTypes: []rules.EventType{rules.MODIFIED},
		},
	}
	rulesObjectSubset = map[rules.RuleID]*rules.Rule{
		"test-rule-1": {
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			EventTypes: []rules.EventType{rules.MODIFIED},
			UpdatesOn:  []string{"spec"},
		},
		"test-rule-2": {
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			EventTypes: []rules.EventType{rules.MODIFIED},
			UpdatesOn:  []string{"metadata"},
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

func waitStartSensor(t *testing.T, sensor *Sensor, ruleSet map[rules.RuleID]*rules.Rule, waitSeconds int) {
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
	}, waitSeconds) {
		t.Error("Failed to start sensor")
		return
	}
}

func setupSensor() *Sensor {
	config := utils.GetKubeAPIConfigOrDie("")
	sensor := New(&Opts{
		KubeConfig: config,
		SensorName: "k8s",
	}, &executor.LogExecutor{})
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
			event := item.(*eventqueue.Event)
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
		}
		retryCount++
		time.Sleep(1 * time.Second)
	}
}

func TestSensorStart(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rulesBasic)
	defer sensor.Stop()
	time.Sleep(3 * time.Second)
	if len(sensor.ruleInformers) != 1 {
		t.Error("Failed to start sensor")
	}
}

func TestSensorReload(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rulesPreReload)
	waitStartSensor(t, sensor, rulesPreReload, 10)

	for ruleID := range rulesPreReload {
		if _, ok := sensor.ruleInformers[ruleID]; !ok {
			t.Errorf("Rule %s should be added", ruleID)
		}
	}

	rule1StartTime := sensor.ruleInformers["test-rule-0"].informerStartTime
	sensor.ReloadRules(rulesReload)
	waitStartSensor(t, sensor, rulesReload, 10)

	if len(rulesReload) != len(sensor.ruleInformers) {
		t.Error("Rules not reloaded properly")
	}
	if _, ok := sensor.ruleInformers["test-rule-2"]; ok {
		t.Error("test-rule-2 should be removed")
	}
	if rule1Inf, ok := sensor.ruleInformers["test-rule-1"]; !ok {
		t.Error("test-rule-1 should be added")
	} else {
		if rule1Inf.rule.EventTypes[0] != rules.ADDED {
			t.Error("test-rule-1 has not been properly updated")
		}
		if rule1Inf.rule.EventTypes[1] != rules.MODIFIED {
			t.Error("test-rule-1 has not been properly updated")
		}
	}
	if rule1StartTime != sensor.ruleInformers["test-rule-0"].informerStartTime {
		t.Error("test-rule-0 should not be touched")
	}

	time.Sleep(1 * time.Second)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "default",
		},
	}
	_, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().ConfigMaps("default").Create(context.Background(), configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create configmap: %s", err)
	}
	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().ConfigMaps("default").Delete(context.Background(), configMap.Name, metav1.DeleteOptions{})
	}()
	switch err := checkIfObjectExistsInQueue(15, sensor, configMap, rules.ADDED); err {
	case errNotFound:
		t.Error("Configmap should be added to queue")
	case errTimeout:
		t.Error("Timeout waiting for configmap to be added to queue")
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
				},
			},
		},
	}
	_, err = kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %s", err)
	}
	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Pods("default").Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	}()
	switch err := checkIfObjectExistsInQueue(15, sensor, pod, rules.ADDED); err {
	case errNotFound:
		t.Error("Pod should be added to queue")
	case errTimeout:
		t.Error("Timeout waiting for pod to be added to queue")
	}
}

func TestObjectsCreatedBeforeSensorStartAreNotAdded(t *testing.T) {
	sensor := setupSensor()
	go sensor.Start(rulesCache)
	defer sensor.Stop()
	waitStartSensor(t, sensor, rulesCache, 10)

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
		t.Fatalf("Failed to create CRD: %v", err)
	}

	time.Sleep(3 * time.Second)

	go sensor.Start(rulesCustom)
	waitStartSensor(t, sensor, rulesCustom, 10)

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
		t.Fatalf("Failed to create CRD instance: %v", err)
	}
	switch checkIfObjectExistsInQueue(30, sensor, crdInst, rules.ADDED) {
	case errNotFound:
		t.Fatal("CRD instance for ADD event not found in queue")
	case errTimeout:
		t.Fatal("Timeout waiting for CRD instance ADD event to be added to queue")
	}
	crdInst.Object["spec"] = "test-spec-modified"
	updatedCrdInst, err := res.Update(context.Background(), crdInst, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update CRD instance: %v", err)
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
	go sensor.Start(rulesClusterbound)
	waitStartSensor(t, sensor, rulesClusterbound, 10)
	fmt.Println("came")
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
		t.Fatalf("Failed to create namespace: %v", err)
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
	go sensor.Start(rulesDynamic)
	waitStartSensor(t, sensor, rulesDynamic, 10)

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
		t.Fatalf("Failed to create namespace: %v", err)
	}

	switch checkIfObjectExistsInQueue(10, sensor, nsObj, rules.ADDED) {
	case nil:
		t.Fatalf("Namespace %s ADDED event should not be added to queue", ns.Name)
	}
	ns.ObjectMeta.Labels = map[string]string{
		"test-label": "test-value",
	}
	nsObj, err = kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Update(context.Background(), ns, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update namespace: %v", err)
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
	go sensor.Start(rulesObjectSubset)
	waitStartSensor(t, sensor, rulesObjectSubset, 10)

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
		t.Fatalf("Failed to create namespace: %v", err)
	}
	ns.ObjectMeta.Labels = map[string]string{
		"test-label": "test-value",
	}
	nsObj, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).CoreV1().Namespaces().Update(context.Background(), ns, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update namespace: %v", err)
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
		t.Fatalf("Failed to create deployment: %v", err)
	}

	defer func() {
		kubernetes.NewForConfigOrDie(sensor.KubeConfig).AppsV1().Deployments(ns.Name).Delete(context.Background(), deployment.Name, metav1.DeleteOptions{})
	}()

	switch checkIfObjectExistsInQueue(5, sensor, deployment, rules.ADDED) {
	case nil:
		t.Fatalf("Deployment %s ADDED event should not be added to queue", deployment.Name)
	}

	deployment.Spec.Template.Spec.Containers[0].Name = "test-container-2-1"
	deploymentObj, err := kubernetes.NewForConfigOrDie(sensor.KubeConfig).AppsV1().Deployments(ns.Name).Update(context.Background(), deployment, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update deployment: %v", err)
	}
	switch checkIfObjectExistsInQueue(5, sensor, deploymentObj, rules.MODIFIED) {
	case errNotFound, errTimeout:
		t.Errorf("Deployment %s MODIFIED event not found in queue", deployment.Name)
	}
}

type mockQueueExecutor struct {
	events []*eventqueue.Event
}

func (mqe *mockQueueExecutor) Execute(event *eventqueue.Event) error {
	mqe.events = append(mqe.events, event)
	return nil
}

func TestWorkerPoolIntegration(t *testing.T) {
	config := utils.GetKubeAPIConfigOrDie("")
	mockExec := &mockQueueExecutor{}
	sensor := New(&Opts{
		KubeConfig: config,
		SensorName: "k8s",
		eventqueueOpts: eventqueue.Opts{
			WorkerCount:  1,
			MaxTryCount:  5,
			RequeueDelay: time.Second * 1,
		},
	}, mockExec)
	go sensor.StartSensorAndWorkerPool(rulesBasic)
	waitStartSensor(t, sensor, rulesBasic, 10)

	// make sure the sensor dint process any old objects
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		if len(mockExec.events) > 0 {
			t.Fatalf("Sensor should not process old objects")
		}
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "int-test-pod-1",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test-container-1",
					Image: "nginx",
				},
			},
		},
	}

	podObj, err := kubernetes.NewForConfigOrDie(config).CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}
	defer func() {
		kubernetes.NewForConfigOrDie(config).CoreV1().Pods("default").Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	}()

	tries := 0
	breakFlag := false
	for {
		tries++
		if tries == 5 {
			t.Fatalf("Pod %s ADDED event not executed", pod.Name)
		}
		if len(mockExec.events) > 1 {
			for _, event := range mockExec.events {
				if event.Objects[0].GetUID() == podObj.GetUID() {
					breakFlag = true
					break
				}
			}
		}
		if breakFlag {
			break
		}
		time.Sleep(time.Second)
	}
	for _, event := range mockExec.events {
		t.Logf("Event description: %s %s", event.EventType, event.Objects[0].GetName())
	}
}

// Total Integration tests for the sensor
// TODO: Add more tests

var (
	executorScript string = `#!/bin/bash
# Check if EVENT environment variable is set
if [ -z "$EVENT" ]; then
	echo "EVENT is not set"
	exit 1
fi

# Try to base64 decode EVENT variable
EVENT_DECODED=$(echo "$EVENT" | base64 -d)
EVENT_DECODED_RULE_ID=$(echo "$EVENT_DECODED" | jq -r '.ruleID')
EVENT_TYPE=$(echo "$EVENT_DECODED" | jq -r '.eventType')
# Check if EVENT_DECODED_RULE_ID is cm-rule-1
if [ "$EVENT_DECODED_RULE_ID" != "cm-rule-1" ]; then
	echo "Rule ID is not cm-rule-1"
	exit 1
fi
echo "$EVENT_TYPE" >> /tmp/int-test-1-1-results.txt
exit 0
`
	rulesConfigMap string = `[{
"id": "cm-rule-1",
"group": "",
"version": "v1",
"resource": "configmaps",
"namespaces": ["default"],
"eventTypes": ["ADDED"]
}]`

	rulesUpdatedConfigMap string = `[{
"id": "cm-rule-1",
"group": "",
"version": "v1",
"resource": "configmaps",
"namespaces": ["default"],
"eventTypes": ["ADDED", "MODIFIED"],
"updatesOn": ["data"]
}]`
)

func TestSensorTotalIntegration(t *testing.T) {
	handleErr := func(err error) {
		if err != nil {
			t.Fatalf("Failed to setup test: %v", err)
		}
	}
	removeFileIfExists := func(filename string) {
		if _, err := os.Stat(filename); err == nil {
			os.Remove(filename)
		}
	}
	processResults := func(resultfile string, expected []string) bool {
		if _, err := os.Stat(resultfile); err == nil {
			file, err := os.Open(resultfile)
			if err != nil {
				return false
			}
			scanner := bufio.NewScanner(file)
			i := 0
			for scanner.Scan() {
				if scanner.Text() != expected[i] {
					return false
				}
				i++
			}
			if i != len(expected) {
				return false
			}
			if err := scanner.Err(); err != nil {
				return false
			}

		} else {
			return false
		}
		return true
	}

	// Remove previous results files if they exist
	// Defer to remove current test results files
	removeFileIfExists("/tmp/int-test-1-1-results.txt")
	defer removeFileIfExists("/tmp/int-test-1-1-results.txt")

	// Create executor script at /tmp/script-cm-rule-1.sh
	err := ioutil.WriteFile("/tmp/script-cm-rule-1.sh", []byte(executorScript), 0755)
	handleErr(err)
	defer os.Remove("/tmp/script-cm-rule-1.sh")

	// Setting up kubernetes namespace and required configmaps
	kubeConfig := utils.GetKubeAPIConfigOrDie("")
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "eventsrunner",
		},
	}

	// Setup test namespace and remove once test is done
	_, err = kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	handleErr(err)
	defer kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})

	// Setup test configmap, will be removed automatically when namespace is removed
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sensor-rules-1",
			Namespace: "eventsrunner",
			Labels: map[string]string{
				"er-k8s-sensor-rules": "true",
			},
		},
		Data: map[string]string{
			"rules": rulesConfigMap,
		},
	}
	_, err = kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().ConfigMaps("eventsrunner").Create(context.Background(), cm, metav1.CreateOptions{})
	handleErr(err)
	configObj, err := config.ParseConfigFromViper("", 1)
	handleErr(err)
	configObj.ExecutorType = "script"
	configObj.ScriptDir = "/tmp"
	configObj.ScriptPrefix = "script"

	// Setup Sensor
	sensorRuntime, err := SetupNewSensorRuntime(configObj)
	defer sensorRuntime.StopSensorRuntime()
	handleErr(err)
	go func() {
		err := sensorRuntime.StartSensorRuntime()
		if err != nil {
			panic(err)
		}
	}()

	// Make sure sensor is running before continuing
	// Try 2 seconds to check if the sensor is running
	if !retryFunc(func() bool {
		return sensorRuntime.GetSensorState() == RUNNING
	}, 2) {
		t.Fatal("Sensor is not running")
	}

	// Rudimentary test to check if the rule was added
	if _, ok := sensorRuntime.sensor.ruleInformers["cm-rule-1"]; !ok {
		t.Fatal("Sensor is not watching cm-rule-1")
	}

	// Making sure the sensor dint execute any executor scripts without
	// events or due to past or zombie events
	if retryFunc(func() bool {
		if _, err := os.Stat("/tmp/int-test-1-1-results.txt"); err == nil {
			return true
		}
		return false
	}, 2) {
		t.Fatal("Sensor executed for past or zombie events")
	}

	// Test event trigger when actual object is added
	// START
	testCm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm1",
			Namespace: "default",
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}
	_, err = kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().ConfigMaps("default").Create(context.Background(), testCm, metav1.CreateOptions{})
	handleErr(err)
	defer kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().ConfigMaps("default").Delete(context.Background(), testCm.Name, metav1.DeleteOptions{})
	if !retryFunc(func() bool {
		return processResults("/tmp/int-test-1-1-results.txt", []string{"added"})
	}, 5) {
		t.Fatal("Sensor dint execute for added event")
	}
	// END

	// Test sensor rule update and check again if the sensor triggered the executor
	// again due to rule update
	// START
	cm.Data = map[string]string{
		"rules": rulesUpdatedConfigMap,
	}
	_, err = kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().ConfigMaps("eventsrunner").Update(context.Background(), cm, metav1.UpdateOptions{})
	handleErr(err)
	if !retryFunc(func() bool {
		return len(sensorRuntime.sensor.ruleInformers["cm-rule-1"].rule.UpdatesOn) == 1
	}, 5) {
		t.Fatal("Sensor should have updated rule")
	}
	if !retryFunc(func() bool {
		return processResults("/tmp/int-test-1-1-results.txt", []string{"added"})
	}, 5) {
		t.Fatal("Sensor executed again on rule update")
	}
	// END

	// Test to make sure sensor dint execute for updates on part of the object
	// not mentioned in the updatedOn rule config field.
	// START
	testCm.ObjectMeta.Labels = map[string]string{
		"test-label": "test-value",
	}
	_, err = kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().ConfigMaps("default").Update(context.Background(), testCm, metav1.UpdateOptions{})
	handleErr(err)
	if retryFunc(func() bool {
		return processResults("/tmp/int-test-1-1-results.txt", []string{"added", "modified"})
	}, 3) {
		t.Fatal("Sensor executed for updates on metadata")
	}
	// END

	// Test if sensor executed for correct update on data field of the object
	// START
	testCm.Data = map[string]string{
		"test-key": "test-value-updated",
	}
	_, err = kubernetes.NewForConfigOrDie(kubeConfig).CoreV1().ConfigMaps("default").Update(context.Background(), testCm, metav1.UpdateOptions{})
	handleErr(err)
	if !retryFunc(func() bool {
		return processResults("/tmp/int-test-1-1-results.txt", []string{"added", "modified"})
	}, 5) {
		t.Fatal("/tmp/int-test-1-1-results.txt should added, modified event in order")
	}
	// END
}

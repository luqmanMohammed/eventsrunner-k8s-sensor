package ruleinformers

import (
	"context"
	"errors"
	"testing"
	"time"

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
	restConfig  = utils.GetKubeAPIConfigOrDie("")
	errNotFound = errors.New("not found")
	errTimeout  = errors.New("timeout")
)

func checkIfObjectExistsInQueue(retry int, eventQueue *eventqueue.EventQueue, searchObject metav1.Object, eventType rules.EventType) error {
	retryCount := 0
	for {
		if eventQueue.Len() > 0 {
			item, shutdown := eventQueue.Get()
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
			eventQueue.Done(item)
		}
		if retryCount == retry {
			return errTimeout
		}
		retryCount++
		time.Sleep(1 * time.Second)
	}
}

func setupInformerFactory() *RuleInformerFactory {
	dynamicClientSet := dynamic.NewForConfigOrDie(restConfig)
	queue := eventqueue.New(
		&executor.LogExecutor{},
		eventqueue.Opts{},
	)
	return NewRuleInformerFactory(dynamicClientSet, "test-sensor", queue)
}

// Confirm informer is working with non namespace resources
var ruleNonNamespaced = rules.Rule{
	ID: "test-rule-1",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	},
	Namespaced: false,
	EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
}

func TestInformerWithNonNamespacedResources(t *testing.T) {
	ruleInformerFactory := setupInformerFactory()
	ruleInformer := ruleInformerFactory.CreateRuleInformer(&ruleNonNamespaced)
	go ruleInformer.Start()

	for !ruleInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
	}()

	nsObj, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	switch checkIfObjectExistsInQueue(30, ruleInformerFactory.queue, nsObj, rules.ADDED) {
	case errNotFound:
		t.Error("Namespace not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for Namespace to be added to queue")
	}
}

// Confirm informer is working with namespace resources
var ruleNamespaced = rules.Rule{
	ID: "test-rule-1",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	},
	Namespaced: true,
	EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
}

func TestInformerWithNamespacedResources(t *testing.T) {
	ruleInformerFactory := setupInformerFactory()
	ruleInformer := ruleInformerFactory.CreateRuleInformer(&ruleNamespaced)
	go ruleInformer.Start()

	for !ruleInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}

	if len(ruleInformer.namespaceInformers) != 1 {
		t.Errorf("Expected 1 namespace informer, got %d", len(ruleInformer.namespaceInformers))
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
					Image: "busybox",
				},
			},
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).CoreV1().Pods(pod.GetNamespace()).Delete(context.Background(), pod.GetName(), metav1.DeleteOptions{})
	}()

	podObj, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Pods(pod.GetNamespace()).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}
	switch checkIfObjectExistsInQueue(10, ruleInformerFactory.queue, podObj, rules.ADDED) {
	case errNotFound:
		t.Error("Namespace not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for Namespace to be added to queue")
	}
}

// Confirm informer is working with namespace resources from different namespaces
var ruleMultiNamespaced = rules.Rule{
	ID: "test-rule-1",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	},
	Namespaces: []string{"default", "eventsrunner"},
	Namespaced: true,
	EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED, rules.DELETED},
}

func TestInformerWithMultiNamespacedResources(t *testing.T) {

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "eventsrunner",
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
	}()

	_, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	ruleInformerFactory := setupInformerFactory()
	ruleInformer := ruleInformerFactory.CreateRuleInformer(&ruleMultiNamespaced)
	go ruleInformer.Start()

	for !ruleInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}

	if len(ruleInformer.namespaceInformers) != 2 {
		t.Fatalf("Expected 2 namespace informers, got %d", len(ruleInformer.namespaceInformers))
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
					Image: "busybox",
				},
			},
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).CoreV1().Pods(pod.GetNamespace()).Delete(context.Background(), pod.GetName(), metav1.DeleteOptions{})
	}()

	podObj, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Pods(pod.GetNamespace()).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}
	switch checkIfObjectExistsInQueue(10, ruleInformerFactory.queue, podObj, rules.ADDED) {
	case errNotFound:
		t.Error("Namespace not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for Namespace to be added to queue")
	}
}

// Confirm only the registered event types are handled
var rulesEventListenerDynamic = rules.Rule{

	GroupVersionResource: schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	},
	EventTypes: []rules.EventType{rules.MODIFIED},
	Namespaced: false,
}

func TestOnlyConfiguredEventListenerIsAdded(t *testing.T) {

	ruleInformerFactory := setupInformerFactory()
	ruleInformer := ruleInformerFactory.CreateRuleInformer(&rulesEventListenerDynamic)
	go ruleInformer.Start()
	for !ruleInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace-2",
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
	}()

	nsObj, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	switch checkIfObjectExistsInQueue(5, ruleInformerFactory.queue, nsObj, rules.ADDED) {
	case nil:
		t.Fatalf("Namespace %s ADDED event should not be added to queue", ns.Name)
	}
	ns.ObjectMeta.Labels = map[string]string{
		"test-label": "test-value",
	}
	nsObj, err = kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Update(context.Background(), ns, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update namespace: %v", err)
	}
	switch checkIfObjectExistsInQueue(30, ruleInformerFactory.queue, nsObj, rules.MODIFIED) {
	case errNotFound:
		t.Errorf("Namespace %s MODIFIED event not found in queue", ns.Name)
	case errTimeout:
		t.Errorf("Timeout waiting for Namespace %s MODIFIED event to be added to queue", ns.Name)
	}
}

// Confirm that a event is created only when a specific object subset is updated
var nsObjSubsetUpdate = rules.Rule{
	ID: "test-1",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	},
	EventTypes: []rules.EventType{rules.MODIFIED},
	UpdatesOn:  []string{"spec"},
	Namespaced: false,
}

var depObjSubsetUpdate = rules.Rule{
	ID: "test-2",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	},
	EventTypes: []rules.EventType{rules.MODIFIED},
	UpdatesOn:  []string{"metadata"},
	Namespaced: true,
}

func TestEnqueueOnlyOnSpecificK8sObjSubsetUpdate(t *testing.T) {
	ruleInformerFactory := setupInformerFactory()
	nsRuleInformer := ruleInformerFactory.CreateRuleInformer(&nsObjSubsetUpdate)
	go nsRuleInformer.Start()
	for !nsRuleInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}
	depRuleInformer := ruleInformerFactory.CreateRuleInformer(&depObjSubsetUpdate)
	go depRuleInformer.Start()
	for !depRuleInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace-3",
		},
	}

	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
	}()
	_, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	ns.ObjectMeta.Labels = map[string]string{
		"test-label": "test-value",
	}
	nsObj, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Namespaces().Update(context.Background(), ns, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update namespace: %v", err)
	}
	switch checkIfObjectExistsInQueue(5, ruleInformerFactory.queue, nsObj, rules.MODIFIED) {
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

	_, err = kubernetes.NewForConfigOrDie(restConfig).AppsV1().Deployments(ns.Name).Create(context.Background(), deployment, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).AppsV1().Deployments(ns.Name).Delete(context.Background(), deployment.Name, metav1.DeleteOptions{})
	}()

	switch checkIfObjectExistsInQueue(2, ruleInformerFactory.queue, deployment, rules.ADDED) {
	case nil:
		t.Fatalf("Deployment %s ADDED event should not be added to queue", deployment.Name)
	}

	deployment.Spec.Template.Spec.Containers[0].Name = "test-container-2-1"
	deploymentObj, err := kubernetes.NewForConfigOrDie(restConfig).AppsV1().Deployments(ns.Name).Update(context.Background(), deployment, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update deployment: %v", err)
	}
	switch checkIfObjectExistsInQueue(5, ruleInformerFactory.queue, deploymentObj, rules.MODIFIED) {
	case errNotFound, errTimeout:
		t.Errorf("Deployment %s MODIFIED event not found in queue", deployment.Name)
	}
}

// Confirm that a event is created when a object from a specific namespace is added, updated, or deleted
var nsFilter = rules.Rule{
	ID: "test-1",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	},
	EventTypes: []rules.EventType{rules.ADDED, rules.ADDED},
	Namespaces: []string{"kube-system"},
	Namespaced: true,
}

func TestEventsOnlyFromConfiguredNamesapcesAreAdded(t *testing.T) {
	ruleInformerFactory := setupInformerFactory()
	filterRuleInformer := ruleInformerFactory.CreateRuleInformer(&nsFilter)
	go filterRuleInformer.Start()
	for !filterRuleInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
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
	defer func() {
		kubernetes.NewForConfigOrDie(restConfig).CoreV1().Pods("default").Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	}()
	_, err := kubernetes.NewForConfigOrDie(restConfig).CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod: %v", err)
	}
	switch checkIfObjectExistsInQueue(3, ruleInformerFactory.queue, pod, rules.ADDED) {
	case nil:
		t.Fatalf("Pod %s ADDED event should not be added to queue", pod.Name)
	}
}

// Confirm informer is working with custom resources
var rulesCustom = rules.Rule{

	ID: "test-rule-1",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "k8ser.io",
		Version:  "v1",
		Resource: "ercrds",
	},
	Namespaces: []string{"default"},
	EventTypes: []rules.EventType{rules.ADDED, rules.MODIFIED},
	Namespaced: true,
}

func TestInformerIsWorkingWithCRDs(t *testing.T) {

	ruleInformerFactory := setupInformerFactory()
	crdInformer := ruleInformerFactory.CreateRuleInformer(&rulesCustom)

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
		clientapiextv1.NewForConfigOrDie(restConfig).CustomResourceDefinitions().Delete(context.Background(), crd.Name, metav1.DeleteOptions{})
	}()

	if _, err := clientapiextv1.NewForConfigOrDie(restConfig).CustomResourceDefinitions().Create(context.Background(), &crd, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Failed to create CRD: %v", err)
	}

	time.Sleep(3 * time.Second)

	go crdInformer.Start()
	for !crdInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}

	crdGVR := schema.GroupVersionResource{
		Group:    "k8ser.io",
		Version:  "v1",
		Resource: "ercrds",
	}

	client := dynamic.NewForConfigOrDie(restConfig)
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
	switch checkIfObjectExistsInQueue(10, ruleInformerFactory.queue, crdInst, rules.ADDED) {
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
	switch checkIfObjectExistsInQueue(10, ruleInformerFactory.queue, updatedCrdInst, rules.MODIFIED) {
	case errNotFound:
		t.Error("CRD instance for MODIFIED event not found in queue")
	case errTimeout:
		t.Error("Timeout waiting for CRD instance MODIFIED event to be added to queue")
	}
}

// Confirm that informer is not adding events from objects added before informer start
var rulePreStart = rules.Rule{
	ID: "test-rule-1",
	GroupVersionResource: schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	},
	Namespaced: false,
	EventTypes: []rules.EventType{rules.ADDED},
}

func TestObjectsCreatedBeforeInformerStartAreNotAdded(t *testing.T) {
	ruleInformerFactory := setupInformerFactory()
	preStartInformer := ruleInformerFactory.CreateRuleInformer(&rulePreStart)
	go preStartInformer.Start()
	for !preStartInformer.stateStarted {
		time.Sleep(1 * time.Second)
	}
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	switch checkIfObjectExistsInQueue(3, ruleInformerFactory.queue, ns, rules.ADDED) {
	case nil:
		t.Errorf("Namespace should not be added")
	}
}

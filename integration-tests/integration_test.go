package integrationtests

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	resourceCount = 500
	interval      = 10 * time.Millisecond
)

var (
	configYaml = `---
sensorNamespace: eventsrunner
executorType: eventsrunner
authType: jwt
jwtToken: test-token
requestTimeout: 10s
`

	ruleConfigYaml = `
[{
    "id": "cm-rule-1",
    "group": "",
    "version": "v1",
    "resource": "configmaps",
    "namespaces": ["k8s-sensor-int-test-ns"],
    "eventTypes": ["ADDED"]
    },{
    "id": "secrets-rule-1",
    "group": "",
    "version": "v1",
    "resource": "secrets",
    "namespaces": ["k8s-sensor-int-test-ns"],
    "eventTypes": ["ADDED"]
	},{
	"id": "cm-rule-2",
	"group": "",
	"version": "v1",
	"resource": "configmaps",
	"namespaces": ["k8s-sensor-int-test-ns-2"],
	"eventTypes": ["ADDED"]
	},{
		"id": "secrets-rule-2",
		"group": "",
		"version": "v1",
		"resource": "secrets",
		"namespaces": ["k8s-sensor-int-test-ns-2"],
		"eventTypes": ["ADDED"]
	}
]
`
	cpuTotal, memoryTotal, maxCPU, maxMem, runCount = 0, 0, 0, 0, 0

	loadGenerators = []*loadGeneratorConfig{
		{
			namespace:     "k8s-sensor-int-test-ns",
			doneChan:      make(chan struct{}),
			generator:     configMapLoadGenerator,
			detectedCount: new(int32),
			ruleID:        "cm-rule-1",
			count:         resourceCount,
			interval:      interval,
		},
		{
			namespace:     "k8s-sensor-int-test-ns",
			doneChan:      make(chan struct{}),
			generator:     secretLoadGenerator,
			detectedCount: new(int32),
			ruleID:        "secrets-rule-1",
			count:         resourceCount,
			interval:      interval,
		},
		{
			namespace:     "k8s-sensor-int-test-ns-2",
			doneChan:      make(chan struct{}),
			generator:     configMapLoadGenerator,
			detectedCount: new(int32),
			ruleID:        "cm-rule-2",
			count:         resourceCount,
			interval:      interval,
		},
		{
			namespace:     "k8s-sensor-int-test-ns-2",
			doneChan:      make(chan struct{}),
			generator:     secretLoadGenerator,
			detectedCount: new(int32),
			ruleID:        "secrets-rule-2",
			count:         resourceCount,
			interval:      interval,
		},
	}
)

type loadGeneratorConfig struct {
	namespace     string
	doneChan      chan struct{}
	detectedCount *int32
	ruleID        string
	count         int
	interval      time.Duration

	generator func(clientSet *kubernetes.Clientset, id, namespace string, count int, interval time.Duration, doneChan chan<- struct{})
}

func (c *loadGeneratorConfig) incrementDetectedCountIfRule(ruleName string) {
	if c.ruleID == ruleName {
		atomic.AddInt32(c.detectedCount, 1)
	}
}

func configMapLoadGenerator(clientSet *kubernetes.Clientset, id, namespace string, count int, interval time.Duration, doneChan chan<- struct{}) {
	for i := 0; i < count; i++ {
		configMapName := fmt.Sprintf("configmap-%s-%d", id, i)
		configMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: namespace,
			},
		}
		_, err := clientSet.CoreV1().ConfigMaps(namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("failed to create configmap %s: %v\n", configMapName, err)
		}
		fmt.Printf("Created configmap: %s\n", configMapName)
		time.Sleep(interval)
	}
	doneChan <- struct{}{}
}

func secretLoadGenerator(clientSet *kubernetes.Clientset, id, namespace string, count int, interval time.Duration, doneChan chan<- struct{}) {
	for i := 0; i < count; i++ {
		secretName := fmt.Sprintf("secret-%s-%d", id, i)
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
		}
		_, err := clientSet.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("failed to create secret %s: %v\n", secretName, err)
		}
		fmt.Printf("Created secret: %s\n", secretName)
		time.Sleep(interval)
	}
	doneChan <- struct{}{}
}

func prepareAndRunJWTBasedMockServer(lgConfigs []*loadGeneratorConfig) {
	mockServerMux := http.NewServeMux()
	mockServerMux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.Header.Get("Authorization") == "Bearer test-token" {
				bodyMap := make(map[string]interface{})
				err := json.NewDecoder(r.Body).Decode(&bodyMap)
				if err != nil {
					fmt.Println("Error decoding request body")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				ruleIDInt, ok := bodyMap["ruleID"]
				if !ok {
					fmt.Println("No ruleId in request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				ruleID := ruleIDInt.(string)
				for _, lgConfig := range lgConfigs {
					lgConfig.incrementDetectedCountIfRule(ruleID)
				}
				w.WriteHeader(http.StatusOK)
			} else {
				fmt.Println("Invalid token")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		} else {
			fmt.Println("Invalid method")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})
	server := &http.Server{
		Addr:    ":9090",
		Handler: mockServerMux,
	}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}

func runShellCommand(t *testing.T, command string) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to execute command %s: %v", command, err)
	}
}

func getCurrentEnvIP(t *testing.T) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("failed to get interfaces: %v", err)
	}
	for _, iface := range ifaces {
		if iface.Name == "eth0" {
			addrs, err := iface.Addrs()
			if err != nil {
				t.Fatalf("failed to get addresses: %v", err)
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						t.Log("VM IP:", ipnet.IP.String())
						return ipnet.IP.String()
					}
				}
			}
		}
	}
	t.Fatal("failed to get VM IP")
	return ""
}

func waitForDeploymentToBeReady(t *testing.T, kubeClient *kubernetes.Clientset, namespace string, deploymentName string, retry int) {
	for i := 0; i < retry; i++ {
		deployment, err := kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get deployment %s: %v", deploymentName, err)
		}
		if deployment.Status.ReadyReplicas == 1 {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("deployment %s is not ready", deploymentName)
}

func collectSensorResourceUsage(config *rest.Config, readyChan chan<- struct{}, stopChan <-chan struct{}) error {
	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		return err
	}
	ready := false
	breakLoop := false
	for true {
		metricsList, err := metricsClient.MetricsV1beta1().PodMetricses("eventsrunner").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=eventsrunner-k8s-sensor",
		})
		if err != nil {
			return err
		}
		if !ready && len(metricsList.Items) != 0 {
			readyChan <- struct{}{}
			ready = true
		}
		for _, podMetric := range metricsList.Items {
			memory, _ := podMetric.Containers[0].Usage.Memory().AsScale(resource.Mega)
			cpuUsage, _ := podMetric.Containers[0].Usage.Cpu().AsScale(resource.Milli)
			memoryMB, _ := memory.AsCanonicalBytes(nil)
			cpuUsageMilli, _ := cpuUsage.AsCanonicalBytes(nil)
			memoryMBInt, _ := strconv.Atoi(string(memoryMB))
			cpuUsageMilliInt, _ := strconv.Atoi(string(cpuUsageMilli))

			memoryTotal += memoryMBInt
			cpuTotal += cpuUsageMilliInt
			runCount++

			if memoryMBInt > maxMem {
				maxMem = memoryMBInt
			}
			if cpuUsageMilliInt > maxCPU {
				maxCPU = cpuUsageMilliInt
			}

			fmt.Printf("CPU: %dm | Memory: %dMi\n", cpuUsageMilliInt, memoryMBInt)
		}
		select {
		case <-stopChan:
			fmt.Println("Stopping sensor resource usage collection")
			breakLoop = true
		default:
			time.Sleep(time.Second)
		}
		if breakLoop {
			break
		}
	}
	avgMem := memoryTotal / runCount
	avgCPU := cpuTotal / runCount
	fmt.Printf("Average CPU: %dm\t Average Memory: %dMi\n", avgCPU, avgMem)
	fmt.Printf("Max CPU: %dm\t Max Memory Usage: %dMi\n", maxCPU, maxMem)
	readyChan <- struct{}{}
	return nil
}

func TestIntegration(t *testing.T) {

	// Skip test if INT_TEST flag is not provided
	if os.Getenv("INT_TEST") != "true" {
		t.Skip("skipping integration test")
	}

	// Run JWT Based mock server in another routine
	go prepareAndRunJWTBasedMockServer(loadGenerators)

	// Setting up prerequisites
	runShellCommand(t, "kubectl create -f prerequisite-k8s-resources.yaml")
	defer runShellCommand(t, "kubectl delete -f prerequisite-k8s-resources.yaml")

	// Get Test Environment IP
	ip := getCurrentEnvIP(t)
	// Create Config Map setting eventsRunner config
	configYaml += "eventsRunnerBaseURL: http://" + ip + ":9090"
	configMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eventsrunner-k8s-sensor-config",
			Namespace: "eventsrunner",
		},
		Data: map[string]string{
			"config.yaml": configYaml,
		},
	}
	kubeconfig := utils.GetKubeAPIConfigOrDie("")
	clientSet := kubernetes.NewForConfigOrDie(kubeconfig)
	_, err := clientSet.CoreV1().ConfigMaps("eventsrunner").Create(context.Background(), &configMap, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create config map: %v", err)
	}
	defer clientSet.CoreV1().ConfigMaps("eventsrunner").Delete(context.Background(), configMap.Name, metav1.DeleteOptions{})

	// Create rules configmap
	rulesConfigYaml := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eventsrunner-rules",
			Namespace: "eventsrunner",
			Labels:    map[string]string{"er-k8s-sensor-rules": "true"},
		},
		Data: map[string]string{
			"rules": ruleConfigYaml,
		},
	}
	_, err = clientSet.CoreV1().ConfigMaps("eventsrunner").Create(context.Background(), &rulesConfigYaml, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create config map: %v", err)
	}
	defer clientSet.CoreV1().ConfigMaps("eventsrunner").Delete(context.Background(), rulesConfigYaml.Name, metav1.DeleteOptions{})

	// Get image tag from IMAGE_TAG env. Will be populated by CI
	imageTag := os.Getenv("IMAGE_TAG")
	if imageTag == "" {
		imageTag = "latest"
	}
	image := "luqmanmohammed/eventsrunner-k8s-sensor:" + imageTag
	t.Logf("Running eventsrunner-k8s-sensor with image %s", image)
	// Read deployment template yaml from senor-deployment.yml file
	var deployment appsv1.Deployment
	deploymentJSON, err := ioutil.ReadFile("sensor-deployment.json")
	if err != nil {
		t.Fatalf("failed to read deployment template file: %v", err)
	}
	err = json.Unmarshal(deploymentJSON, &deployment)
	if err != nil {
		t.Fatalf("failed to unmarshal deployment template file: %v", err)
	}
	deployment.Spec.Template.Spec.Containers[0].Image = image
	// Create sensor deployment
	_, err = clientSet.AppsV1().Deployments("eventsrunner").Create(context.Background(), &deployment, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create deployment: %v", err)
	}
	defer clientSet.AppsV1().Deployments("eventsrunner").Delete(context.Background(), deployment.Name, metav1.DeleteOptions{})
	// Wait for deployment to be ready
	waitForDeploymentToBeReady(t, clientSet, "eventsrunner", deployment.Name, 30)

	// Setup monitoring
	metricsReadyChan := make(chan struct{})
	metricsStopChan := make(chan struct{})
	go func() {
		if err := collectSensorResourceUsage(kubeconfig, metricsReadyChan, metricsStopChan); err != nil {
			panic(err)
		}
	}()
	select {
	case <-metricsReadyChan:
		t.Log("Metrics are ready")
	case <-time.After(time.Minute * 5):
		t.Fatalf("failed to initialize sensor resource usage")
	}

	loadGenStartTime := time.Now()

	concurrency := 20
	for i, config := range loadGenerators {
		configConcCount := config.count / concurrency
		config.count = configConcCount * concurrency
		for j := 0; j < concurrency; j++ {
			go config.generator(clientSet, fmt.Sprintf("%d-%d", i, j), config.namespace, configConcCount, config.interval, config.doneChan)
		}
	}

	for _, config := range loadGenerators {
		for i := 0; i < concurrency; i++ {
			<-config.doneChan
		}
	}

	t.Logf("Load generation took %s", time.Since(loadGenStartTime))

	// Calculate how much time is required extra to finish processing
	startTime := time.Now()

	for {
		processedGenerators := 0
		proccessedItems := 0
		for _, config := range loadGenerators {
			if config.count == int(*config.detectedCount) {
				processedGenerators++
				proccessedItems += int(*config.detectedCount)
			}
		}
		if processedGenerators == len(loadGenerators) {
			t.Logf("All generators processed %d items", proccessedItems)
			t.Logf("Extra time taken after last event was created %v", time.Since(startTime))
			break
		}
		if time.Since(startTime) > time.Minute*1 {
			t.Fatal("Failed to process all events within extra minute")
		}
		time.Sleep(time.Millisecond)
	}
	metricsStopChan <- struct{}{}
	<-metricsReadyChan
}

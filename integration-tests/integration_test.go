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

var cmRuleCount, secretsRuleCount = new(int32), new(int32)

const RESOURCE_COUNT = 100

func PrepareAndRunJWTBasedMockServer() {
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
				if ruleID == "cm-rule-1" {
					fmt.Println("ConfigMap Added")
					atomic.AddInt32(cmRuleCount, 1)
				} else if ruleID == "secrets-rule-1" {
					fmt.Println("Secrets Added")
					atomic.AddInt32(secretsRuleCount, 1)
				}
				fmt.Println("Rule ID:", ruleID)
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

func RunShellCommand(t *testing.T, command string) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to execute command %s: %v", command, err)
	}
}

func GetCurrentEnvIP(t *testing.T) string {
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

func WaitForDeploymentToBeReady(t *testing.T, kubeClient *kubernetes.Clientset, namespace string, deploymentName string, retry int) {
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

var cpuMC, memoryMC = make([]int, 0), make([]int, 0)

func CollectSensorResourceUsage(config *rest.Config, readyChan chan struct{}, stopChan chan struct{}) error {
	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		return err
	}
	ready := false
	for true {
		metricsList, err := metricsClient.MetricsV1beta1().PodMetricses("eventsrunner").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=eventsrunner-k8s-sensor",
		})
		if err != nil {
			return err
		}
		fmt.Printf("Metrics count: %d\n", len(metricsList.Items))
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
			memoryMC = append(memoryMC, memoryMBInt)
			cpuMC = append(cpuMC, cpuUsageMilliInt)
			fmt.Printf("Metrics CPU Usage: %dm\t Memory Usage: %dMi\n", cpuUsageMilliInt, memoryMBInt)
		}
		select {
		case <-stopChan:
			fmt.Println("Stopping sensor resource usage collection")
			break
		default:
			time.Sleep(time.Second)
		}
	}
	avgMem, avgCPU := 0, 0
	maxCPU, maxMem := 0, 0
	for _, mem := range memoryMC {
		if mem > maxMem {
			maxMem = mem
		}
		avgMem += mem
	}
	for _, cpu := range cpuMC {
		if cpu > maxCPU {
			maxCPU = cpu
		}
		avgCPU += cpu
	}
	avgMem = avgMem / len(memoryMC)
	avgCPU = avgCPU / len(cpuMC)
	fmt.Printf("Average Metrics CPU Usage: %dm\t Average Memory Usage: %dMi\n", avgCPU, avgMem)
	fmt.Printf("Max Metrics CPU Usage: %dm\t Max Memory Usage: %dMi\n", maxCPU, maxMem)
	return nil
}

var configYaml = `---
sensorNamespace: eventsrunner
executorType: eventsrunner
authType: jwt
jwtToken: test-token
requestTimeout: 10s
`

var ruleConfigYaml = `
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
}]
`

var sensorDeployment = appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "eventsrunner-k8s-sensor",
		Namespace: "eventsrunner",
	},
	Spec: appsv1.DeploymentSpec{
		Replicas: &[]int32{1}[0],
	},
}

func TestIntegration(t *testing.T) {

	if os.Getenv("INT_TEST") != "true" {
		t.Skip("skipping integration test")
	}

	go PrepareAndRunJWTBasedMockServer()

	// Setting up prerequisites
	RunShellCommand(t, "kubectl create -f prerequisite-k8s-resources.yaml")
	defer RunShellCommand(t, "kubectl delete -f prerequisite-k8s-resources.yaml")

	// Get Test Environment IP
	ip := GetCurrentEnvIP(t)

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
	WaitForDeploymentToBeReady(t, clientSet, "eventsrunner", deployment.Name, 30)

	metricsReadyChan := make(chan struct{})
	metricsStopChan := make(chan struct{})
	go func() {
		if err := CollectSensorResourceUsage(kubeconfig, metricsReadyChan, metricsStopChan); err != nil {
			panic(err)
		}
	}()

	select {
	case <-metricsReadyChan:
		t.Log("Metrics are ready")
	case <-time.After(time.Second * 30):
		t.Fatalf("failed to initialize sensor resource usage")
	}

	cmDone, secretDone := make(chan struct{}), make(chan struct{})
	go func(done chan struct{}) {
		for i := 0; i < RESOURCE_COUNT; i++ {
			// Create configmap
			testCM := v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cm-" + strconv.Itoa(i),
					Namespace: "k8s-sensor-int-test-ns",
				},
				Data: map[string]string{},
			}
			_, err = clientSet.CoreV1().ConfigMaps("k8s-sensor-int-test-ns").Create(context.Background(), &testCM, metav1.CreateOptions{})
			if err != nil {
				fmt.Printf("failed to create configmap: %v\n", err)
			}
			time.Sleep(time.Millisecond * 100)
		}
		close(done)
	}(cmDone)

	go func(done chan struct{}) {
		for i := 0; i < RESOURCE_COUNT; i++ {
			testSecret := v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-" + strconv.Itoa(i),
					Namespace: "k8s-sensor-int-test-ns",
				},
				Data: map[string][]byte{},
			}
			_, err = clientSet.CoreV1().Secrets("k8s-sensor-int-test-ns").Create(context.Background(), &testSecret, metav1.CreateOptions{})
			if err != nil {
				fmt.Printf("failed to create secret: %v\n", err)
			}
			time.Sleep(time.Millisecond * 100)
		}
		close(done)
	}(secretDone)

	<-cmDone
	<-secretDone

	metricsStopChan <- struct{}{}
	time.Sleep(time.Second * 30)

	if *cmRuleCount != int32(RESOURCE_COUNT) {
		t.Fatalf("expected 10 configmaps, got %d", *cmRuleCount)
	}

	if *secretsRuleCount != int32(RESOURCE_COUNT) {
		t.Fatalf("expected 10 secrets, got %d", *secretsRuleCount)
	}
}

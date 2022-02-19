package collector

import (
	"context"
	"testing"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	exampleBasicRuleStr = `
	[{
		"id": "basic-pod-rule",
		"group": "",
		"version": "v1",
		"resource": "pods",
		"namespaces": ["default"],
		"eventTypes": ["ADDED", "MODIFIED"]
	}]
	`
	exampleBasicRule2Str = `
	[{
		"id": "basic-configmap-rule",
		"group": "",
		"version": "v1",
		"resource": "configmaps",
		"namespaces": ["default"],
		"eventTypes": ["ADDED", "MODIFIED"]
	},{
		"id": "basic-namespace-rule",
		"group": "",
		"version": "v1",
		"resource": "namespaces",
		"eventTypes": ["ADDED"]
	},{
		"id": "basic-pod-rule",
		"group": "",
		"version": "v1",
		"resource": "pods",
		"namespaces": ["default"],
		"eventTypes": ["ADDED", "MODIFIED"]
	}]
	`
)

func setupRuleCollector() *ConfigMapRuleCollector {
	config := utils.GetKubeAPIConfigOrDie("")
	return NewConfigMapRuleCollector(kubernetes.NewForConfigOrDie(config), "default", "er-sensor-rules=true")
}

func addRuleConfigMap(configMapName string, strRule string) error {
	config := utils.GetKubeAPIConfigOrDie("")
	if _, err := kubernetes.NewForConfigOrDie(config).CoreV1().ConfigMaps("default").Create(context.Background(), &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
			Labels: map[string]string{
				"er-sensor-rules": "true",
			},
		},
		Data: map[string]string{
			"rules": strRule,
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}
	return nil
}

func deleteRuleConfigMap(configMapName string) error {
	config := utils.GetKubeAPIConfigOrDie("")
	if err := kubernetes.NewForConfigOrDie(config).CoreV1().ConfigMaps("default").Delete(context.Background(), configMapName, metav1.DeleteOptions{}); err != nil {
		return err
	}
	return nil
}

func TestStarterRuleCollectionFromMultipleConfigMaps(t *testing.T) {
	addRuleConfigMap("basic-rules", exampleBasicRuleStr)
	defer deleteRuleConfigMap("basic-rules")
	addRuleConfigMap("basic-rules2", exampleBasicRule2Str)
	defer deleteRuleConfigMap("basic-rules2")
	ruleCollector := setupRuleCollector()
	if collectedRules, err := ruleCollector.Collect(context.Background()); err != nil {
		t.Errorf("Error while collecting rules: %v", err)
	} else {
		if len(collectedRules) != 3 {
			t.Fatalf("Expected 3 rules, got %d", len(collectedRules))
		}
		if len(collectedRules["basic-configmap-rule"].EventTypes) != 2 {
			t.Fatalf("Expected 2 event types, got %d", len(collectedRules["basic-configmap-rule"].EventTypes))
		}
		if collectedRules["basic-namespace-rule"].EventTypes[0] != rules.ADDED {
			t.Fatalf("Expected event type %s, got %s", rules.ADDED, collectedRules["basic-namespace-rule"].EventTypes[0])
		}
	}
}

type mockServer struct {
	Rules map[rules.RuleID]*rules.Rule
}

func (ms *mockServer) ReloadRules(sensorRules map[rules.RuleID]*rules.Rule) {
	ms.Rules = sensorRules
}

func TestContinuosRuleCollection(t *testing.T) {
	ms := &mockServer{}
	checkRules := func(ruleID rules.RuleID, retry int) bool {
		retryCount := 0
		for {
			if len(ms.Rules) > 0 {
				if _, ok := ms.Rules[ruleID]; ok {
					return true
				}
			}
			if retryCount >= retry {
				return false
			}
			retryCount++
			time.Sleep(1 * time.Second)
		}
	}
	addRuleConfigMap("basic-rules", exampleBasicRuleStr)
	defer deleteRuleConfigMap("basic-rules")
	ruleCollector := setupRuleCollector()
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go ruleCollector.StartRuleCollector(ctx, ms)
	if !checkRules("basic-pod-rule", 10) {
		t.Fatal("Timeout trying to find basic-pod-rule")
	}
	addRuleConfigMap("basic-rules-2", exampleBasicRule2Str)
	defer deleteRuleConfigMap("basic-rules-2")
	if !checkRules("basic-namespace-rule", 10) {
		t.Fatal("Timeout trying to find basic-namespace-rule")
	}
}

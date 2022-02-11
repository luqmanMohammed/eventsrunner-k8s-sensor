package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules/collector/validator"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// SensorReloadInterface should be implemented by sensors which needs to
// support automatic reloading of rules.
type SensorReloadInterface interface {
	ReloadRules(sensorRules map[rules.RuleID]*rules.Rule)
}

// ConfigMapRuleCollector is used to collect rules from a kubernetes configmaps.
// Rules will be collect from the configmaps in the sensor namespace with the
// label provided via the sensorRuleConfigMapLabel field.
type ConfigMapRuleCollector struct {
	clientSet                *kubernetes.Clientset
	sensorNamespace          string
	sensorRuleConfigMapLabel string
}

// NewConfigMapRuleCollector returns a new ConfigMapRuleCollector.
func NewConfigMapRuleCollector(clientSet *kubernetes.Clientset, sensorNamespace string, sensorRuleConfigMapLabel string) *ConfigMapRuleCollector {
	return &ConfigMapRuleCollector{
		clientSet:                clientSet,
		sensorNamespace:          sensorNamespace,
		sensorRuleConfigMapLabel: sensorRuleConfigMapLabel,
	}
}

// Collect will return all rules from the configmaps in the sensor namespace
// with the label configured via sensorRuleConfigMapLabel.
// Only one copy of the duplicate rules will be returned.
// If the configmap does not exist or the rules are invalid, an empty map will
// be returned.
// If unable to list all configmaps in the sensor namespace, an error will be
// returned.
func (cmrc ConfigMapRuleCollector) Collect(ctx context.Context) (map[rules.RuleID]*rules.Rule, error) {
	klog.V(1).Infof("Collecting rules from %v namespace with label %v", cmrc.sensorNamespace, cmrc.sensorRuleConfigMapLabel)
	cmList, err := cmrc.clientSet.CoreV1().ConfigMaps(cmrc.sensorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: cmrc.sensorRuleConfigMapLabel,
	})
	if err != nil {
		klog.V(1).ErrorS(err, "Error trying to list configmaps")
		return nil, err
	}
	klog.V(1).Infof("Number of configmaps to process: %d", len(cmList.Items))
	rules := cmrc.parseCollectedConfigMapsIntoRules(cmList.Items)
	return rules, nil
}

// parseCollectedConfigMapsIntoRules parses provided list of configmaps into
// a map of rules.
// Data field of configmap should contain key `rules`, which should contain a json
// list of valid rules (Refer rules.Rule struct doc for further information).
// Each rule should contain a valid rule ID, else rule wont be processed.
// If any errors are found during json decoding, error will be logged and the
// function will continue executing.
func (cmrc ConfigMapRuleCollector) parseCollectedConfigMapsIntoRules(cmList []v1.ConfigMap) map[rules.RuleID]*rules.Rule {
	parsedRules := make(map[rules.RuleID]*rules.Rule)
	for _, cm := range cmList {
		tmpRules := make([]*rules.Rule, 0)
		rulesStr, ok := cm.Data["rules"]
		if !ok {
			klog.V(2).Infof("Collected configmap %v, doesn't contain 'rules' key. Skipping", cm.Name)
			continue
		}
		if errJSONUnMarshal := json.Unmarshal([]byte(rulesStr), &tmpRules); errJSONUnMarshal != nil {
			klog.V(2).ErrorS(errJSONUnMarshal, fmt.Sprintf("JSON error when decoding content of configmap %v. Skipping", cm.Name))
			continue
		}
		for _, rule := range tmpRules {
			switch validator.NormalizeAndValidateRule(rule) {
			case nil:
				klog.V(3).Infof("Rule %v is valid", rule.ID)
			case validator.ErrRuleIDNotFound:
				klog.V(3).ErrorS(validator.ErrRuleIDNotFound, fmt.Sprintf("Rule %v doesn't have an ID. Skipping", rule.ID))
				continue
			case validator.ErrRuleEventTypesNotFound:
				klog.V(3).ErrorS(validator.ErrRuleEventTypesNotFound, fmt.Sprintf("Rule %v doesn't have an event types list. Skipping", rule.ID))
				continue
			case validator.ErrRuleEventTypesNotValid:
				klog.V(3).ErrorS(validator.ErrRuleEventTypesNotValid, fmt.Sprintf("Rule %v has an invalid event types list. Should be one of added, modified or deleted. Skipping", rule.ID))
				continue
			}
			if _, ok := parsedRules[rule.ID]; !ok {
				parsedRules[rule.ID] = rule
				klog.V(3).Infof("Rule %v successfully loaded from configmap %v", rule.ID, cm.Name)
			}
		}
	}
	return parsedRules
}

// StartRuleCollector will continuously listen for rule configmap changes and
// will reload the rules of the provided sensor.
// Cancelling the provided context will stop the collector.
func (cmrc ConfigMapRuleCollector) StartRuleCollector(ctx context.Context, sensorRI SensorReloadInterface) {
	parseStoreIntoConfigMaps := func(store cache.Store) []v1.ConfigMap {
		cmList := make([]v1.ConfigMap, len(store.ListKeys()))
		for i, cm := range store.List() {
			cmList[i] = *cm.(*v1.ConfigMap)
		}
		return cmList
	}

	klog.V(1).Info("Configuring continuos rule collector")
	changeQueue := make(chan struct{}, 10)
	informerFactory := informers.NewSharedInformerFactoryWithOptions(cmrc.clientSet, 0, informers.WithNamespace(cmrc.sensorNamespace), informers.WithTweakListOptions(func(lo *metav1.ListOptions) {
		lo.LabelSelector = cmrc.sensorRuleConfigMapLabel
	}))
	informerFactory.Core().V1().ConfigMaps().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			changeQueue <- struct{}{}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			changeQueue <- struct{}{}
		},
		DeleteFunc: func(obj interface{}) {
			changeQueue <- struct{}{}
		},
	})
	klog.V(1).Info("Starting continuos rule collector")
	go informerFactory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informerFactory.Core().V1().ConfigMaps().Informer().HasSynced) {
		klog.V(2).ErrorS(errors.New("ConfigMap Cache Wait Failed"), "")
	}
	for {
		select {
		case <-ctx.Done():
			klog.V(1).Info("Stopping rule collector")
			return
		case <-changeQueue:
			klog.V(2).Info("Rule change detected. Parsing and reloading rules")
			configMapList := parseStoreIntoConfigMaps(informerFactory.Core().V1().ConfigMaps().Informer().GetStore())
			ruleMap := cmrc.parseCollectedConfigMapsIntoRules(configMapList)
			sensorRI.ReloadRules(ruleMap)
		}
	}
}

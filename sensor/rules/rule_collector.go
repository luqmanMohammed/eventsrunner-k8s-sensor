package rules

import (
	"context"
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type RuleCollector interface{}

type ConfigMapRuleCollector struct {
	clientSet                *kubernetes.Clientset
	sensorNamespace          string
	sensorRuleConfigMapLabel string
}

func (cmrc ConfigMapRuleCollector) Collect(ctx context.Context) ([]*Rule, error) {
	rules := []*Rule{}
	ruleSet := map[string]struct{}{}
	cmList, err := cmrc.clientSet.CoreV1().ConfigMaps(cmrc.sensorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: cmrc.sensorRuleConfigMapLabel,
	})
	for _, cm := range cmList.Items {
		tmpRules := []*Rule{}
		rulesStr, ok := cm.Data["rules"]
		if !ok {
			continue
		}
		if errJsonUnMarshal := json.Unmarshal([]byte(rulesStr), &tmpRules); errJsonUnMarshal != nil {
			continue
		}
		for _, rule := range tmpRules {
			if rule.Name == "" {
				continue
			}
			if _, ok := ruleSet[rule.Name]; !ok {
				rules = append(rules, rule)
				ruleSet[rule.Name] = struct{}{}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return rules, nil
}

func (cmrc ConfigMapRuleCollector) StartCollector() {
	configMapQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	factory := informers.NewSharedInformerFactoryWithOptions(cmrc.clientSet, 0, informers.WithNamespace(cmrc.sensorNamespace), informers.WithTweakListOptions(func(options *metav1.ListOptions) {
		options.LabelSelector = cmrc.sensorRuleConfigMapLabel
	}))
	factory.Core().V1().ConfigMaps().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			configMap := obj.(*v1.ConfigMap)
			if namespaceKey, err := cache.MetaNamespaceKeyFunc(configMap); err != nil {
				klog.Errorf("Could not get key for configmap %s: %v", configMap.Name, err)
			} else {
				configMapQueue.Add(namespaceKey)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			configMap := newObj.(*v1.ConfigMap)
			if namespaceKey, err := cache.MetaNamespaceKeyFunc(configMap); err != nil {
				klog.Errorf("Could not get key for configmap %s: %v", configMap.Name, err)
			} else {
				configMapQueue.Add(namespaceKey)
			}
		},
		DeleteFunc: func(obj interface{}) {
			configMap := obj.(*v1.ConfigMap)
			if namespaceKey, err := cache.DeletionHandlingMetaNamespaceKeyFunc(configMap); err != nil {
				klog.Errorf("Could not get key for configmap %s: %v", configMap.Name, err)
			} else {
				configMapQueue.Add(namespaceKey)
			}
		},
	})
	stopChan := make(chan struct{})
	go factory.Start(stopChan)
	if cache.WaitForCacheSync(stopChan, factory.Core().V1().ConfigMaps().Informer().HasSynced) {
		klog.Info("ConfigMap informer synced")
	} else {
		klog.Error("ConfigMap informer did not sync")
	}

	<-stopChan
}

package ruleinformers

import (
	"fmt"
	"reflect"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// RuleInformerFactory is a helper struct to facilitate creation of RuleInformers
type RuleInformerFactory struct {
	dynamicClientSet dynamic.Interface
	sensorName       string
	queue            *eventqueue.EventQueue
}

// NewRuleInformerFactory creates a new RuleInformerFactory.
func NewRuleInformerFactory(dynamicClientSet dynamic.Interface, sensorName string, queue *eventqueue.EventQueue) *RuleInformerFactory {
	return &RuleInformerFactory{
		dynamicClientSet: dynamicClientSet,
		sensorName:       sensorName,
		queue:            queue,
	}
}

// CreateRuleInformer creates a new informer for the provide rule.
// Informers filters will be configured according to the rule.
// Label rules with <SensorName>=ignore for the event to be ignored.
// TODO: Add namespace wide ignore by adding <SensorName>=ignore to the
// 	     namespace label.
func (rif *RuleInformerFactory) CreateRuleInformer(rule *rules.Rule) *RuleInformer {
	var indexers map[string]cache.IndexFunc
	if rule.Namespaced {
		if len(rule.Namespaces) == 0 {
			indexers = cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
		} else {
			indexers = cache.Indexers{}
		}
	} else {
		indexers = cache.Indexers{}
	}

	tweakFunction := dynamicinformer.TweakListOptionsFunc(func(options *metav1.ListOptions) {
		labelSeclector := fmt.Sprintf("%s!=ignore", rif.sensorName)
		if rule.LabelFilter != "" {
			labelSeclector += "," + rule.LabelFilter
		}
		options.LabelSelector = labelSeclector
		options.FieldSelector = rule.FieldFilter
	})

	ruleStopChan := make(chan struct{})
	ruleInformer := &RuleInformer{
		Rule:              rule,
		stopChan:          ruleStopChan,
		InformerStartTime: time.Now().Local().Truncate(time.Second),
	}

	createNsInformer := func(ns string) informers.GenericInformer {
		nsInformer := dynamicinformer.NewFilteredDynamicInformer(
			rif.dynamicClientSet,
			rule.GroupVersionResource,
			ns,
			0,
			indexers,
			tweakFunction,
		)
		if ns == "" {
			ns = "all"
		}
		klog.V(3).Infof("Registering event handler for rule %v for namespace %s", rule.ID, ns)
		nsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    rif.addFuncWrapper(rule, ruleInformer.InformerStartTime),
			UpdateFunc: rif.updateFuncWrapper(rule),
			DeleteFunc: rif.deleteFuncWrapper(rule),
		})
		return nsInformer
	}

	var nsInformers []informers.GenericInformer
	if len(rule.Namespaces) == 0 {
		nsInformers = make([]informers.GenericInformer, 0, 1)
		nsInformers = append(nsInformers, createNsInformer(metav1.NamespaceAll))
	} else {
		nsInformers = make([]informers.GenericInformer, 0, len(rule.Namespaces))
		for _, ns := range rule.Namespaces {
			nsInformers = append(nsInformers, createNsInformer(ns))
		}
	}

	ruleInformer.namespaceInformers = nsInformers

	klog.V(2).Infof("Registered Informers for rule %v", rule.ID)
	return ruleInformer
}

// addFuncWrapper injects the rules into the add resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// ADDED event type is not configured for a specific rule.
// If the objects where created before the start of the rule, the event wont be
// processed.
func (rif *RuleInformerFactory) addFuncWrapper(rule *rules.Rule, startTime time.Time) func(obj interface{}) {
	for _, tEventType := range rule.EventTypes {
		if tEventType == rules.ADDED {
			return func(obj interface{}) {
				unstructuredObj := obj.(*unstructured.Unstructured)
				if !unstructuredObj.GetCreationTimestamp().After(startTime) {
					klog.V(4).Infof("Object %v was created before the start of the rule %v", unstructuredObj.GetName(), rule.ID)
					return
				}
				klog.V(4).Infof("Adding object %v:%v to the event queue for the ADDED event", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				rif.queue.Add(&eventqueue.Event{
					EventType: rules.ADDED,
					RuleID:    rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj},
				})
			}
		}
	}
	klog.V(2).Infof("ADDED event type is not configured for rule %v", rule.ID)
	return nil
}

// updateFuncWrapper injects the rules into the update resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// MODIFIED event type is not configured for a specific rule.
// If the resource version of both new and old objects are same, the event
// wont be processed.
// Old object is stored as primary at index 0 and new object as secondary at index 1.
// TODO: Add more in depth checks for updatesOn filter
func (rif *RuleInformerFactory) updateFuncWrapper(rule *rules.Rule) func(obj interface{}, newObj interface{}) {
	for _, tEventType := range rule.EventTypes {
		if tEventType == rules.MODIFIED {
			return func(obj interface{}, newObj interface{}) {

				unstructuredObj := obj.(*unstructured.Unstructured)
				unstructuredNewObj := newObj.(*unstructured.Unstructured)

				if unstructuredNewObj.GetResourceVersion() == unstructuredObj.GetResourceVersion() {
					klog.V(4).Infof("Actual update for object %s:%s was not detected", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
					return
				}

				if len(rule.UpdatesOn) > 0 {
					klog.V(4).Infof("Event on object %s:%s is subjected to updates on filter", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
					enqueue := false
					for _, updateOn := range rule.UpdatesOn {
						updateOnStr := string(updateOn)
						if !reflect.DeepEqual(unstructuredObj.Object[updateOnStr], unstructuredNewObj.Object[updateOnStr]) {
							enqueue = true
							break
						}
					}
					if !enqueue {
						klog.V(4).Infof("Event on object %s:%s will not be processed since configured subset was not updated", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
						return
					}
				}

				klog.V(4).Infof("Adding object %v:%v to the event queue for the MODIFIED event", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				rif.queue.Add(&eventqueue.Event{
					EventType: rules.MODIFIED,
					RuleID:    rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj, unstructuredNewObj},
				})
			}
		}
	}
	klog.V(2).Infof("MODIFIED event type is not configured for rule %v", rule.ID)
	return nil
}

// deleteFuncWrapper injects the rules into the delete resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// DELETED event type is not configured for a specific rule.
func (rif *RuleInformerFactory) deleteFuncWrapper(rule *rules.Rule) func(obj interface{}) {
	for _, tEventType := range rule.EventTypes {
		if tEventType == rules.DELETED {
			return func(obj interface{}) {
				unstructuredObj := obj.(*unstructured.Unstructured)
				klog.V(4).Infof("Adding object %v:%v to the event queue for the DELETED event", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				rif.queue.Add(&eventqueue.Event{
					EventType: rules.DELETED,
					RuleID:    rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj},
				})
			}
		}
	}
	klog.V(2).Infof("DELETED event type is not configured for rule %v", rule.ID)
	return nil
}

// RuleInformer holds information related to a rule in runtime
// along with the informer that is responsible for listening to the
// events for a specific rule.
// Closing the stopChan channel will stop the informer which prevents
// events for the specific rule from being collected.
type RuleInformer struct {
	Rule               *rules.Rule
	InformerStartTime  time.Time
	namespaceInformers []informers.GenericInformer
	stopChan           chan struct{}
}

// Start starts the informer for the rule.
// It will start all relevant kubernetes informers in all configured namespaces.
// Blocks until all informer caches are synced.
// Closing the stopChan channel by calling the Stop function will all the informers.
func (ri *RuleInformer) Start() {
	klog.V(3).Infof("Starting informer for rule %v", ri.Rule.ID)
	for _, nsInf := range ri.namespaceInformers {
		go nsInf.Informer().Run(ri.stopChan)
	}
	informersHasSynced := make([]cache.InformerSynced, 0, len(ri.namespaceInformers))
	for _, informer := range ri.namespaceInformers {
		informersHasSynced = append(informersHasSynced, informer.Informer().HasSynced)
	}
	if !cache.WaitForCacheSync(ri.stopChan, informersHasSynced...) {
		klog.V(1).Infof("Error occurred while waiting for informers to sync for rule %v", ri.Rule.ID)
	}
	klog.V(3).Infof("Informer for rule %v started", ri.Rule.ID)
}

// Stop stops the informer for the rule.
func (ri *RuleInformer) Stop() {
	klog.V(3).Infof("Informer for rule %v stopped", ri.Rule.ID)
	close(ri.stopChan)
}

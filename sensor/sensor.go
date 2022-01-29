package sensor

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// ruleInformer holds information related to a rule in runtime
// along with the informer that is responsible to listen to the
// events for a specific rule.
// closing the stopChan channel will stop the informer and the
// informer will stop listening to the events for the said rule.
type ruleInformer struct {
	rule              *rules.Rule
	informerStartTime time.Time
	informer          informers.GenericInformer
	stopChan          chan struct{}
}

// startInformer starts the informer for a specific rule.
func (rr *ruleInformer) startInformer() {
	go rr.informer.Informer().Run(rr.stopChan)
}

// SensorOpts holds options related to sensor configuration
type SensorOpts struct {
	eventqueue.EventQueueOpts
	KubeConfig                     *rest.Config
	SensorLabel                    string
	LoadObjectsDurationBeforeStart time.Duration
}

// Sensor struct implements kubernetes informers to sense changes
// according to the rules defined.
// Responsible for managing all informers and event queue
type Sensor struct {
	*SensorOpts
	dynamicClientSet dynamic.Interface
	Queue            *eventqueue.EventQueue
	ruleInformers    map[rules.RuleID]*ruleInformer
	stopChan         chan struct{}
	lock             sync.Mutex
}

// Creates a new default Sensor. Refer Sensor struct documentation for
// more information.
// SensorOpts is required.
func New(sensorOpts *SensorOpts, executor eventqueue.QueueExecutor) *Sensor {
	if sensorOpts == nil {
		panic("SensorOpts cannot be nil")
	}
	dynamicClientSet := dynamic.NewForConfigOrDie(sensorOpts.KubeConfig)
	return &Sensor{
		SensorOpts:       sensorOpts,
		dynamicClientSet: dynamicClientSet,
		ruleInformers:    make(map[rules.RuleID]*ruleInformer),
		stopChan:         make(chan struct{}),
		Queue:            eventqueue.New(executor, sensorOpts.EventQueueOpts),
	}
}

// addFuncWrapper injects the rules into the add resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// ADDED event type is not configured for a specific rule.
// If the objects where created before the start of the rule, the event wont be
// processed.
func (s *Sensor) addFuncWrapper(ruleInf *ruleInformer) func(obj interface{}) {
	for _, t_eventType := range ruleInf.rule.EventTypes {
		if t_eventType == rules.ADDED {
			return func(obj interface{}) {
				unstructuredObj := obj.(*unstructured.Unstructured)
				if !unstructuredObj.GetCreationTimestamp().After(ruleInf.informerStartTime) {
					return
				}
				s.Queue.Add(&eventqueue.Event{
					EventType: rules.ADDED,
					RuleID:    ruleInf.rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj},
				})
			}
		}
	}
	klog.V(4).Infof("ADDED event type is not configured for rule %v", ruleInf.rule.ID)
	return nil
}

// updateFuncWrapper injects the rules into the update resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// MODIFIED event type is not configured for a specific rule.
// If the resource version of both new and old objects are same, the event
// wont be processed.
// Old object is stored as primary at index 0 and new object as secoundry at index 1.
func (s *Sensor) updateFuncWrapper(ruleInf *ruleInformer) func(obj interface{}, newObj interface{}) {
	for _, t_eventType := range ruleInf.rule.EventTypes {
		if t_eventType == rules.MODIFIED {
			return func(obj interface{}, newObj interface{}) {

				unstructuredObj := obj.(*unstructured.Unstructured)
				unstructuredNewObj := newObj.(*unstructured.Unstructured)

				if unstructuredNewObj.GetResourceVersion() == unstructuredObj.GetResourceVersion() {
					return
				}

				if len(ruleInf.rule.UpdatesOn) > 0 {
					enqueue := false
					for _, updateOn := range ruleInf.rule.UpdatesOn {
						updateOnStr := string(updateOn)
						if !reflect.DeepEqual(unstructuredObj.Object[updateOnStr], unstructuredNewObj.Object[updateOnStr]) {
							enqueue = true
							break
						}
					}
					if !enqueue {
						return
					}
				}

				s.Queue.Add(&eventqueue.Event{
					EventType: rules.MODIFIED,
					RuleID:    ruleInf.rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj, unstructuredNewObj},
				})
			}
		}
	}
	klog.V(4).Infof("MODIFIED event type is not configured for rule %v", ruleInf.rule.ID)
	return nil
}

// deleteFuncWrapper injects the rules into the delete resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// DELETED event type is not configured for a specific rule.
func (s *Sensor) deleteFuncWrapper(ruleInf *ruleInformer) func(obj interface{}) {
	for _, t_eventType := range ruleInf.rule.EventTypes {
		if t_eventType == rules.DELETED {
			return func(obj interface{}) {
				s.Queue.Add(&eventqueue.Event{
					EventType: rules.DELETED,
					RuleID:    ruleInf.rule.ID,
					Objects:   []*unstructured.Unstructured{obj.(*unstructured.Unstructured)},
				})
			}
		}
	}
	klog.V(4).Infof("DELETED event type is not configured for rule %v", ruleInf.rule.ID)
	return nil
}

// ReloadRules will reload affected sensor rules without requiring a restart.
// Thread safe by using mutex.
// Calculates which of the rules are affected, and reloads them.
// Added new rules which are not present in the old rules will be added.
// Rules which are not present in the new rules will be removed.
// ReloadRules assumes all rules are valid and are unique.
func (s *Sensor) ReloadRules(sensorRules map[rules.RuleID]*rules.Rule) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for newRuleId, newRule := range sensorRules {
		if oldRuleInformer, ok := s.ruleInformers[newRuleId]; !ok {
			ruleInf := s.registerInformerForRule(newRule)
			s.ruleInformers[newRuleId] = ruleInf
			ruleInf.startInformer()
		} else {
			if !reflect.DeepEqual(oldRuleInformer.rule, newRule) {
				close(oldRuleInformer.stopChan)
				ruleInf := s.registerInformerForRule(newRule)
				s.ruleInformers[newRuleId] = ruleInf
				ruleInf.startInformer()
			}
		}
	}
	for oldRuleId, oldRuleInformer := range s.ruleInformers {
		if _, ok := sensorRules[oldRuleId]; !ok {
			close(oldRuleInformer.stopChan)
			delete(s.ruleInformers, oldRuleId)
		}
	}
}

// registerInformerForRule creates a new informer for the provide rule.
// Informers filters will be configured according to the rule.
// TODO: Give more meaningful labelSelector.
func (s *Sensor) registerInformerForRule(rule *rules.Rule) *ruleInformer {
	dynamicInformer := dynamicinformer.NewFilteredDynamicInformer(
		s.dynamicClientSet,
		rule.GroupVersionResource,
		metav1.NamespaceAll,
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		dynamicinformer.TweakListOptionsFunc(func(options *metav1.ListOptions) {
			labelSeclector := fmt.Sprintf("er-%s!=false", s.SensorLabel)
			if rule.LabelFilter != "" {
				labelSeclector += "," + rule.LabelFilter
			}
			options.LabelSelector = labelSeclector
			options.FieldSelector = rule.FieldFilter
		}))

	klog.V(3).Infof("Registering event handler for rule %v", rule.ID)

	ruleStopChan := make(chan struct{})
	ruleInformer := &ruleInformer{
		rule:              rule,
		informer:          dynamicInformer,
		stopChan:          ruleStopChan,
		informerStartTime: time.Now().Local(),
	}

	dynamicInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			klog.V(5).Infof("FilterFunc called for rule %v with object %v", rule.ID, obj)
			meta, ok := obj.(metav1.Object)
			if !ok {
				return false
			}
			if len(rule.Namespaces) != 0 && !utils.StringInSlice(meta.GetNamespace(), rule.Namespaces) {
				return false
			}
			return true
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    s.addFuncWrapper(ruleInformer),
			UpdateFunc: s.updateFuncWrapper(ruleInformer),
			DeleteFunc: s.deleteFuncWrapper(ruleInformer),
		},
	})
	klog.V(2).Infof("Registered Informers for rule %v", rule.ID)
	return ruleInformer
}

// Start starts the sensor. It will start all informers which will register event handlers
// and filters based on the rules.
// Start wont validate rules for uniques.
// Start is a blocking call.
func (s *Sensor) Start(sensorRules map[rules.RuleID]*rules.Rule) {
	klog.V(1).Info("Starting sensor")
	for ruleID, rule := range sensorRules {
		ruleInformer := s.registerInformerForRule(rule)
		ruleInformer.startInformer()
		s.ruleInformers[ruleID] = ruleInformer
	}
	<-s.stopChan
}

// StartSensorAndWorkerPool will start the sensor and the worker pool.
// Worker pool which is part of the eventqueue module will consume events from teh queue.
func (s *Sensor) StartSensorAndWorkerPool(sensorRules map[rules.RuleID]*rules.Rule) {
	go s.Start(sensorRules)
	go s.Queue.StartQueueWorkerPool()
	<-s.stopChan
}

// Stop stops the sensor. It will stop all informers and unregister event handlers.
// Stop will block until all informers are stopped.
// Stop will release Start call.
func (s *Sensor) Stop() {
	klog.V(1).Info("Stopping sensor")
	for _, ruleInf := range s.ruleInformers {
		close(ruleInf.stopChan)
	}
	close(s.stopChan)
	s.Queue.ShutDownWithDrain()
}

package sensor

import (
	"fmt"
	"sync"

	"github.com/luqmanMohammed/er-k8s-sensor/common"
	"github.com/luqmanMohammed/er-k8s-sensor/sensor/rules"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// Event struct holds all information related to an event.
// TODO: Move to another package
type Event struct {
	EventType rules.EventType
	Rule      *rules.Rule
	Objects   []metav1.Object
}

// registeredRule is a struct wrapper arround rules.Rule.
// It holds required rule management objects.
type registeredRule struct {
	rule            *rules.Rule
	ruleK8sInformer dynamicinformer.DynamicSharedInformerFactory
	stopChan        chan struct{}
}

// startInformer starts the informer for a specific rule.
func (rr *registeredRule) startInformer() {
	rr.ruleK8sInformer.Start(rr.stopChan)
}

// SensorOpts holds options related to sensor configuration
type SensorOpts struct {
	KubeConfig  *rest.Config
	SensorLabel string
}

// Sensor struct implements kubernetes informers to sense changes
// according to the rules defined.
// Responsible for managing all informers and event queue
type Sensor struct {
	*SensorOpts
	dynamicClientSet dynamic.Interface
	Queue            workqueue.RateLimitingInterface
	registeredRules  []registeredRule
	stopChan         chan struct{}
	lock             sync.Mutex
}

// Creates a new default Sensor. Refer Sensor struct documentation for
// more information.
// SensorOpts is required.
func New(sensorOpts *SensorOpts) *Sensor {
	if sensorOpts == nil {
		panic("SensorOpts cannot be nil")
	}
	dynamicClientSet := dynamic.NewForConfigOrDie(sensorOpts.KubeConfig)
	return &Sensor{
		SensorOpts:       sensorOpts,
		dynamicClientSet: dynamicClientSet,
		registeredRules:  make([]registeredRule, 0),
		stopChan:         make(chan struct{}),
		Queue:            workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
}

// addFuncWrapper wraps inject the rules into the add resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// ADDED event type is not configured for a specific rule.
func (s *Sensor) addFuncWrapper(rule *rules.Rule) func(obj interface{}) {
	for _, t_eventType := range rule.EventTypes {
		if t_eventType == rules.ADDED {
			return func(obj interface{}) {
				s.Queue.Add(&Event{
					EventType: rules.ADDED,
					Rule:      rule,
					Objects:   []metav1.Object{obj.(metav1.Object)},
				})
			}
		}
	}
	klog.V(4).Infof("ADDED event type is not configured for rule %v", rule)
	return nil
}

// updateFuncWrapper wraps inject the rules into the update resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// MODIFIED event type is not configured for a specific rule.
// Old object is stored as primary at index 0 and new object as secoundry at index 1.
func (s *Sensor) updateFuncWrapper(rule *rules.Rule) func(obj interface{}, newObj interface{}) {
	for _, t_eventType := range rule.EventTypes {
		if t_eventType == rules.MODIFIED {
			return func(obj interface{}, newObj interface{}) {
				s.Queue.Add(&Event{
					EventType: rules.MODIFIED,
					Rule:      rule,
					Objects:   []metav1.Object{obj.(metav1.Object), newObj.(metav1.Object)},
				})
			}
		}
	}
	klog.V(4).Infof("MODIFIED event type is not configured for rule %v", rule)
	return nil
}

// deleteFuncWrapper wraps inject the rules into the delete resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// DELETED event type is not configured for a specific rule.
func (s *Sensor) deleteFuncWrapper(rule *rules.Rule) func(obj interface{}) {
	for _, t_eventType := range rule.EventTypes {
		if t_eventType == rules.DELETED {
			return func(obj interface{}) {
				s.Queue.Add(&Event{
					EventType: rules.DELETED,
					Rule:      rule,
					Objects:   []metav1.Object{obj.(metav1.Object)},
				})
			}
		}
	}
	klog.V(4).Infof("DELETED event type is not configured for rule %v", rule)
	return nil
}

// ReloadRules will reload affected sensor rules without requiring a restart.
// Thread safe.
// Preps new rules, closes all informers for the existing rules and starts new
// informers for the new rules.
// ReloadRules assumes all rules are valid and are unique.
func (s *Sensor) ReloadRules(sensorRules []*rules.Rule) {
	newRegisteredRules := make([]registeredRule, 0)
	for _, rule := range sensorRules {
		newRegisteredRules = append(newRegisteredRules, s.registerRule(rule))
	}
	defer s.lock.Unlock()
	s.lock.Lock()
	for _, rule := range s.registeredRules {
		close(rule.stopChan)
	}
	for _, rule := range newRegisteredRules {
		rule.startInformer()
	}
	s.registeredRules = newRegisteredRules
}

func (s *Sensor) registerRule(rule *rules.Rule) registeredRule {
	dyInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(s.dynamicClientSet, 0, metav1.NamespaceAll, dynamicinformer.TweakListOptionsFunc(func(options *metav1.ListOptions) {
		labelSeclector := fmt.Sprintf("er-%s!=false", s.SensorLabel)
		if rule.LabelFilter != "" {
			labelSeclector += "," + rule.LabelFilter
		}
		options.LabelSelector = labelSeclector
		options.FieldSelector = rule.FieldFilter
	}))
	resInformer := dyInformerFactory.ForResource(schema.GroupVersionResource{
		Group:    rule.Group,
		Version:  rule.APIVersion,
		Resource: rule.Resource,
	}).Informer()

	klog.V(3).Infof("Registering event handler for rule %v", rule)

	resInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			klog.V(5).Infof("FilterFunc called for rule %v with object %v", rule, obj)
			meta, ok := obj.(metav1.Object)
			if !ok {
				return false
			}
			if len(rule.Namespaces) != 0 && !common.StringInSlice(meta.GetNamespace(), rule.Namespaces) {
				return false
			}
			return true
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    s.addFuncWrapper(rule),
			UpdateFunc: s.updateFuncWrapper(rule),
			DeleteFunc: s.deleteFuncWrapper(rule),
		},
	})
	klog.V(2).Infof("Registered Informers for rule %v", rule)
	ruleStopChan := make(chan struct{})
	return registeredRule{
		rule:            rule,
		stopChan:        ruleStopChan,
		ruleK8sInformer: dyInformerFactory,
	}
}

// Start starts the sensor. It will start all informers and register event handlers
// and filters based on the rules.
// Start wont validate rules for unniqness.
// Start is a blocking call.
func (s *Sensor) Start(sensorRules []*rules.Rule) {
	klog.V(1).Info("Starting sensor")
	for _, rule := range sensorRules {
		r_rule := s.registerRule(rule)
		r_rule.startInformer()
		s.registeredRules = append(s.registeredRules, r_rule)
	}
	<-s.stopChan
}

// Stop stops the sensor. It will stop all informers and unregister event handlers.
// Stop will block until all informers are stopped.
// Stop will release Start call.
func (s *Sensor) Stop() {
	klog.V(1).Info("Stopping sensor")
	for _, rRule := range s.registeredRules {
		close(rRule.stopChan)
	}
	close(s.stopChan)
}

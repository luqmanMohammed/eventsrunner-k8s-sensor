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
	dynamicClientSet         dynamic.Interface
	Queue                    workqueue.RateLimitingInterface
	dynamicInformerFactories []*dynamicinformer.DynamicSharedInformerFactory
	StopChan                 chan struct{}
	lock                     sync.Mutex
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
		SensorOpts:               sensorOpts,
		dynamicClientSet:         dynamicClientSet,
		dynamicInformerFactories: make([]*dynamicinformer.DynamicSharedInformerFactory, 0),
		StopChan:                 make(chan struct{}),
		Queue:                    workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
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
func (s *Sensor) ReloadRules(sensorRules []*rules.Rule) error {
	defer s.lock.Unlock()
	s.lock.Lock()
	return nil
}

// Start starts the sensor. It will start all informers and register event handlers
// and filters based on the rules.
// Start wont validate rules for unniqness.
// Start is a blocking call.
func (s *Sensor) Start(sensorRules []*rules.Rule) {
	klog.V(1).Info("Starting sensor")
	for _, t_rule := range sensorRules {
		dyInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(s.dynamicClientSet, 0, metav1.NamespaceAll, dynamicinformer.TweakListOptionsFunc(func(options *metav1.ListOptions) {
			labelSeclector := fmt.Sprintf("er-%s!=false", s.SensorLabel)
			if t_rule.LabelFilter != "" {
				labelSeclector += "," + t_rule.LabelFilter
			}
			options.LabelSelector = labelSeclector
			options.FieldSelector = t_rule.FieldFilter
		}))
		res_informer := dyInformerFactory.ForResource(schema.GroupVersionResource{
			Group:    t_rule.Group,
			Version:  t_rule.APIVersion,
			Resource: t_rule.Resource,
		}).Informer()

		klog.V(3).Infof("Registering event handler for rule %v", t_rule)

		res_informer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				klog.V(5).Infof("FilterFunc called for rule %v with object %v", t_rule, obj)
				meta, ok := obj.(metav1.Object)
				if !ok {
					return false
				}
				if len(t_rule.Namespaces) != 0 && !common.StringInSlice(meta.GetNamespace(), t_rule.Namespaces) {
					return false
				}
				return true
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    s.addFuncWrapper(t_rule),
				UpdateFunc: s.updateFuncWrapper(t_rule),
				DeleteFunc: s.deleteFuncWrapper(t_rule),
			},
		})
		klog.V(2).Infof("Registered Informers for rule %v", t_rule)
		s.dynamicInformerFactories = append(s.dynamicInformerFactories, &dyInformerFactory)
		dyInformerFactory.Start(s.StopChan)
	}
	<-s.StopChan
}

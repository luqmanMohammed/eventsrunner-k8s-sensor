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
)

type Event struct {
	EventType rules.EventType
	Rule      *rules.Rule
	Objects   []metav1.Object
}

type SensorOpts struct {
	KubeConfig  *rest.Config
	SensorLabel string
}

type Sensor struct {
	*SensorOpts
	dynamicClientSet         dynamic.Interface
	Queue                    workqueue.RateLimitingInterface
	dynamicInformerFactories []*dynamicinformer.DynamicSharedInformerFactory
	StopChan                 chan struct{}
	lock                     sync.Mutex
}

func New(sensorOpts *SensorOpts) *Sensor {
	dynamicClientSet := dynamic.NewForConfigOrDie(sensorOpts.KubeConfig)
	return &Sensor{
		SensorOpts:               sensorOpts,
		dynamicClientSet:         dynamicClientSet,
		dynamicInformerFactories: make([]*dynamicinformer.DynamicSharedInformerFactory, 0),
		StopChan:                 make(chan struct{}),
		Queue:                    workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
}

func (s *Sensor) addFuncDecorator(rule *rules.Rule) func(obj interface{}) {
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
	return nil
}

func (s *Sensor) updateFuncDecorator(rule *rules.Rule) func(obj interface{}, newObj interface{}) {
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
	return nil
}

func (s *Sensor) deleteFuncDecorator(rule *rules.Rule) func(obj interface{}) {
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
	return nil
}

func (s *Sensor) ReloadRules(sensorRules []*rules.Rule) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.StopChan <- struct{}{}
	s.dynamicInformerFactories = make([]*dynamicinformer.DynamicSharedInformerFactory, 0)
	s.StopChan = make(chan struct{})
	s.Start(sensorRules)
	return nil
}

func (s *Sensor) Start(sensorRules []*rules.Rule) {
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

		res_informer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
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
				AddFunc:    s.addFuncDecorator(t_rule),
				UpdateFunc: s.updateFuncDecorator(t_rule),
				DeleteFunc: s.deleteFuncDecorator(t_rule),
			},
		})
		s.dynamicInformerFactories = append(s.dynamicInformerFactories, &dyInformerFactory)
		dyInformerFactory.Start(s.StopChan)
	}
	<-s.StopChan
}

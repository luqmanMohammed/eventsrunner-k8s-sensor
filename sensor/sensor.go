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
)

type OverideEventFunctionOpts struct {
	AddFunc    func(obj interface{})
	UpdateFunc func(obj interface{}, newObj interface{})
	DeleteFunc func(obj interface{})
}

type SensorOpts struct {
	KubeConfig  *rest.Config
	SensorLabel string
}

type Sensor struct {
	*SensorOpts
	*OverideEventFunctionOpts
	dynamicClientSet         dynamic.Interface
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
	}
}

func (s *Sensor) AddFunc(obj interface{}) {

}

func (s *Sensor) UpdateFunc(obj, newObj interface{}) {

}

func (s *Sensor) DeleteFunc(obj interface{}) {

}

func (s *Sensor) ReloadRules(sensorRules *[]rules.Rule) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.StopChan <- struct{}{}
	s.dynamicInformerFactories = make([]*dynamicinformer.DynamicSharedInformerFactory, 0)
	s.StopChan = make(chan struct{})
	s.Start(sensorRules)
	return nil
}

func (s *Sensor) Start(sensorRules *[]rules.Rule) {
	for _, t_rule := range *sensorRules {
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
				AddFunc: func() func(obj interface{}) {
					if s.OverideEventFunctionOpts != nil {
						return s.OverideEventFunctionOpts.AddFunc
					}
					for _, t_eventType := range t_rule.EventTypes {
						if t_eventType == rules.ADDED {
							return s.AddFunc
						}
					}
					return nil
				}(),
				UpdateFunc: func() func(obj interface{}, newObj interface{}) {
					if s.OverideEventFunctionOpts != nil {
						return s.OverideEventFunctionOpts.UpdateFunc
					}
					for _, t_eventType := range t_rule.EventTypes {
						if t_eventType == rules.ADDED {
							return s.UpdateFunc
						}
					}
					return nil
				}(),
				DeleteFunc: func() func(obj interface{}) {
					if s.OverideEventFunctionOpts != nil {
						return s.OverideEventFunctionOpts.DeleteFunc
					}
					for _, t_eventType := range t_rule.EventTypes {
						if t_eventType == rules.ADDED {
							return s.DeleteFunc
						}
					}
					return nil
				}(),
			},
		})
		s.dynamicInformerFactories = append(s.dynamicInformerFactories, &dyInformerFactory)
		dyInformerFactory.Start(s.StopChan)
	}
	<-s.StopChan
}

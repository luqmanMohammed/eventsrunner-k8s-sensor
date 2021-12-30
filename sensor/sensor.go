package sensor

import (
	"fmt"

	"github.com/luqmanMohammed/er-k8s-sensor/common"
	"github.com/luqmanMohammed/er-k8s-sensor/sensor/rules"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type SensorOpts struct {
	KubeConfig  *rest.Config
	SensorLabel string
}

type Sensor struct {
	*SensorOpts
	dynamicClientSet         dynamic.Interface
	dynamicInformerFactories map[*rules.Rule]*dynamicinformer.DynamicSharedInformerFactory
	stopChan                 chan struct{}
}

func New(sensorOpts *SensorOpts) *Sensor {
	dynamicClientSet := dynamic.NewForConfigOrDie(sensorOpts.KubeConfig)
	return &Sensor{
		SensorOpts:               sensorOpts,
		dynamicClientSet:         dynamicClientSet,
		dynamicInformerFactories: make(map[*rules.Rule]*dynamicinformer.DynamicSharedInformerFactory),
		stopChan:                 make(chan struct{}),
	}
}

func (s Sensor) AddFunc(obj interface{}) {
}

func (s Sensor) UpdateFunc(obj, newObj interface{}) {
}

func (s Sensor) DeleteFunc(obj interface{}) {
}

func (s *Sensor) ReloadRules(sensorRules *[]rules.Rule) error {
	s.stopChan <- struct{}{}
	s.dynamicInformerFactories = make(map[*rules.Rule]*dynamicinformer.DynamicSharedInformerFactory)
	s.stopChan = make(chan struct{})
	return s.Start(sensorRules)
}

func (s *Sensor) Start(sensorRules *[]rules.Rule) error {
	for _, t_rule := range *sensorRules {
		dyInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(s.dynamicClientSet, 0, metav1.NamespaceAll, dynamicinformer.TweakListOptionsFunc(func(options *metav1.ListOptions) {
			options.LabelSelector = fmt.Sprintf("er-%s!=false,%s", s.SensorLabel, t_rule.LabelFilter)
			options.FieldSelector = t_rule.FieldFilter
		}))
		res_informer := dyInformerFactory.ForResource(schema.GroupVersionResource{
			Group:    t_rule.Group,
			Version:  t_rule.APIVersion,
			Resource: t_rule.Resource,
		}).Informer()
		res_informer.AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				meta, ok := obj.(metav1.ObjectMeta)
				if !ok {
					return false
				}
				if t_rule.Namespaces != nil && !common.StringInSlice(meta.Namespace, t_rule.Namespaces) {
					return false
				}
				return true
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func() func(obj interface{}) {
					for _, t_eventType := range t_rule.EventTypes {
						if t_eventType == rules.ADDED {
							return s.AddFunc
						}
					}
					return nil
				}(),
				UpdateFunc: func() func(obj interface{}, newObj interface{}) {
					for _, t_eventType := range t_rule.EventTypes {
						if t_eventType == rules.ADDED {
							return s.UpdateFunc
						}
					}
					return nil
				}(),
				DeleteFunc: func() func(obj interface{}) {
					for _, t_eventType := range t_rule.EventTypes {
						if t_eventType == rules.ADDED {
							return s.DeleteFunc
						}
					}
					return nil
				}(),
			},
		})
		s.dynamicInformerFactories[&t_rule] = &dyInformerFactory
		dyInformerFactory.Start(s.stopChan)
	}
	return nil
}

package sensor

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/config"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner/client"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules/collector"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// SensorState type depicts the state of the sensor.
type SensorState int

const (
	STARTING SensorState = iota
	RUNNING
	STOPPING
	STOPPED
)

// ruleInformer holds information related to a rule in runtime
// along with the informer that is responsible for listening to the
// events for a specific rule.
// Closing the stopChan channel will stop the informer which prevents
// events for the specific rule from being collected.
type ruleInformer struct {
	rule              *rules.Rule
	informerStartTime time.Time
	informer          informers.GenericInformer
	stopChan          chan struct{}
}

// startInformer starts the informer for a specific rule.
func (rr *ruleInformer) startInformer() {
	klog.V(2).Infof("Starting informer for rule %v", rr.rule.ID)
	go rr.informer.Informer().Run(rr.stopChan)
}

// SensorOpts holds options related to sensor configuration
// - KubConfig : kubernetes config
// - EventQueueOpts : event queue options. Refer eventqueue.EventQueueOpts
// - SensorName: name of the sensor
type SensorOpts struct {
	eventqueue.EventQueueOpts
	KubeConfig *rest.Config
	SensorName string
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
	state            SensorState
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
					klog.V(4).Infof("Object %v was created before the start of the rule %v", unstructuredObj.GetName(), ruleInf.rule.ID)
					return
				}
				klog.V(4).Infof("Adding object %v:%v to the event queue for the ADDED event", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				s.Queue.Add(&eventqueue.Event{
					EventType: rules.ADDED,
					RuleID:    ruleInf.rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj},
				})
			}
		}
	}
	klog.V(2).Infof("ADDED event type is not configured for rule %v", ruleInf.rule.ID)
	return nil
}

// updateFuncWrapper injects the rules into the update resource event handler
// function without affecting its signature.
// Makes event handler addition dynamic based on the rules by returning nil if
// MODIFIED event type is not configured for a specific rule.
// If the resource version of both new and old objects are same, the event
// wont be processed.
// Old object is stored as primary at index 0 and new object as secondary at index 1.
func (s *Sensor) updateFuncWrapper(ruleInf *ruleInformer) func(obj interface{}, newObj interface{}) {
	for _, t_eventType := range ruleInf.rule.EventTypes {
		if t_eventType == rules.MODIFIED {
			return func(obj interface{}, newObj interface{}) {

				unstructuredObj := obj.(*unstructured.Unstructured)
				unstructuredNewObj := newObj.(*unstructured.Unstructured)

				if unstructuredNewObj.GetResourceVersion() == unstructuredObj.GetResourceVersion() {
					klog.V(4).Infof("Actual update for object %s:%s was not detected", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
					return
				}

				if len(ruleInf.rule.UpdatesOn) > 0 {
					klog.V(4).Infof("Event on object %s:%s is subjected to updates on filter", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
					enqueue := false
					for _, updateOn := range ruleInf.rule.UpdatesOn {
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
				s.Queue.Add(&eventqueue.Event{
					EventType: rules.MODIFIED,
					RuleID:    ruleInf.rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj, unstructuredNewObj},
				})
			}
		}
	}
	klog.V(2).Infof("MODIFIED event type is not configured for rule %v", ruleInf.rule.ID)
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
				unstructuredObj := obj.(*unstructured.Unstructured)
				klog.V(4).Infof("Adding object %v:%v to the event queue for the DELETED event", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				s.Queue.Add(&eventqueue.Event{
					EventType: rules.DELETED,
					RuleID:    ruleInf.rule.ID,
					Objects:   []*unstructured.Unstructured{unstructuredObj},
				})
			}
		}
	}
	klog.V(2).Infof("DELETED event type is not configured for rule %v", ruleInf.rule.ID)
	return nil
}

// ReloadRules will reload affected sensor rules without requiring a restart.
// Thread safe by using mutex.
// Finds out which of the rules are affected, and reloads only them.
// Newly added rules which are not present in the old rules will be added.
// Rules which are not present in the new rules will be removed.
// For the rules which were updated, old informer will stopped and a new one
// will be created with the new rule configuration.
// If the sensor is not in a Running state, rules will not be reloaded.
// ReloadRules assumes all rules are valid and are unique.
func (s *Sensor) ReloadRules(sensorRules map[rules.RuleID]*rules.Rule) {
	s.lock.Lock()
	defer s.lock.Unlock()
	klog.V(1).Infof("Reloading rules for sensor %v", s.SensorName)
	if s.state != RUNNING {
		klog.V(1).Info("Sensor is not running, skipping reloading rules")
		return
	}
	for newRuleId, newRule := range sensorRules {
		if oldRuleInformer, ok := s.ruleInformers[newRuleId]; !ok {
			klog.V(2).Infof("Rule %v is not present in the old rules, adding it", newRuleId)
			ruleInf := s.registerInformerForRule(newRule)
			s.ruleInformers[newRuleId] = ruleInf
			ruleInf.startInformer()
		} else {
			if !reflect.DeepEqual(oldRuleInformer.rule, newRule) {
				klog.V(2).Infof("Rule %v is present in the old rules, but the configuration is different, updating it", newRuleId)
				close(oldRuleInformer.stopChan)
				ruleInf := s.registerInformerForRule(newRule)
				s.ruleInformers[newRuleId] = ruleInf
				ruleInf.startInformer()
			} else {
				klog.V(2).Infof("Rule %s is not changed, skipping reloading", newRuleId)
			}
		}
	}
	for oldRuleId, oldRuleInformer := range s.ruleInformers {
		if _, ok := sensorRules[oldRuleId]; !ok {
			klog.V(2).Infof("Rule %v is not present in the new rules, removing it", oldRuleId)
			close(oldRuleInformer.stopChan)
			delete(s.ruleInformers, oldRuleId)
		}
	}
}

// registerInformerForRule creates a new informer for the provide rule.
// Informers filters will be configured according to the rule.
// Label rules with <SensorName>=ignore for the event to be ignored.
// TODO: Add namespace wide ignore by adding <SensorName>=ignore to the
// 	     namespace label.
func (s *Sensor) registerInformerForRule(rule *rules.Rule) *ruleInformer {
	dynamicInformer := dynamicinformer.NewFilteredDynamicInformer(
		s.dynamicClientSet,
		rule.GroupVersionResource,
		metav1.NamespaceAll,
		0,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		dynamicinformer.TweakListOptionsFunc(func(options *metav1.ListOptions) {
			labelSeclector := fmt.Sprintf("%s!=ignore", s.SensorName)
			if rule.LabelFilter != "" {
				labelSeclector += "," + rule.LabelFilter
			}
			options.LabelSelector = labelSeclector
			options.FieldSelector = rule.FieldFilter
		}))

	klog.V(1).Infof("Registering event handler for rule %v", rule.ID)

	ruleStopChan := make(chan struct{})
	ruleInformer := &ruleInformer{
		rule:              rule,
		informer:          dynamicInformer,
		stopChan:          ruleStopChan,
		informerStartTime: time.Now().Local(),
	}

	dynamicInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			meta, ok := obj.(metav1.Object)
			klog.V(4).Infof("FilterFunc called for rule %v with object %v:%v", rule.ID, meta.GetNamespace(), meta.GetName())
			if !ok {
				return false
			}
			if len(rule.Namespaces) != 0 && !utils.StringInSlice(meta.GetNamespace(), rule.Namespaces) {
				klog.V(4).Infof("FilterFunc: Namespace %v is not in the list of namespaces %v for object %v:%v", meta.GetNamespace(), rule.Namespaces, meta.GetNamespace(), meta.GetName())
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
// Start assumes rules are valid and unique.
// Start is a blocking call, it will block until the sensor is stopped.
func (s *Sensor) Start(sensorRules map[rules.RuleID]*rules.Rule) {
	klog.V(1).Info("Starting sensor")
	s.state = STARTING
	for ruleID, rule := range sensorRules {
		ruleInformer := s.registerInformerForRule(rule)
		ruleInformer.startInformer()
		s.ruleInformers[ruleID] = ruleInformer
	}
	s.state = RUNNING
	<-s.stopChan
}

// StartSensorAndWorkerPool will start the sensor and the worker pool.
// Worker pool which is part of the eventqueue module will consume events from the queue.
func (s *Sensor) StartSensorAndWorkerPool(sensorRules map[rules.RuleID]*rules.Rule) {
	go s.Start(sensorRules)
	go s.Queue.StartQueueWorkerPool()
	<-s.stopChan
}

// Stop stops the sensor. It will stop all informers which will unregister all
// event handlers.
// Stop will block until all informers are stopped.
func (s *Sensor) Stop() {
	s.state = STOPPING
	klog.V(1).Info("Stopping sensor")
	for _, ruleInf := range s.ruleInformers {
		close(ruleInf.stopChan)
	}
	close(s.stopChan)
	klog.V(1).Info("Stopped all informers, draining queue")
	s.Queue.ShutDownWithDrain()
	s.state = STOPPED
}

// SensorRuntime sets up the sensor runtime and manages it.
// TODO: Rework cancelFunc in rule collectors
type SensorRuntime struct {
	sensor        *Sensor
	ruleCollector *collector.ConfigMapRuleCollector
	cancelFunc    context.CancelFunc
}

func (sr *SensorRuntime) GetSensorState() SensorState {
	klog.V(3).Infof("Current sensor state: %v", sr.sensor.state)
	return sr.sensor.state
}

// SetupSensorRuntime will setup the sensor and return a sensor runtime.
// SetupSensor will collect the required KubeConfig and initialize the sensor
// to be able to start.
func SetupNewSensorRuntime(sensorConfig *config.Config) (*SensorRuntime, error) {
	kubeConfig, err := utils.GetKubeAPIConfig(sensorConfig.KubeConfigPath)
	if err != nil {
		klog.V(2).ErrorS(err, "Error when try to create kube config")
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		klog.V(2).ErrorS(err, "Error when try to create kube client")
		return nil, err
	}
	ruleCollector := collector.NewConfigMapRuleCollector(kubeClient, sensorConfig.SensorNamespace, sensorConfig.SensorRuleConfigMapLabel)
	executor, err := executor.New(
		executor.ExecutorType(sensorConfig.ExecutorType),
		executor.ExecutorOpts{
			ScriptDir:    sensorConfig.ScriptDir,
			ScriptPrefix: sensorConfig.ScriptPrefix,
			AuthType:     client.AuthType(sensorConfig.AuthType),
			EventsRunnerClientOpts: client.EventsRunnerClientOpts{
				EventsRunnerBaseURL: sensorConfig.EventsRunnerBaseURL,
				CaCertPath:          sensorConfig.CaCertPath,
				ClientCertPath:      sensorConfig.ClientCertPath,
				ClientKeyPath:       sensorConfig.ClientKeyPath,
				JWTToken:            sensorConfig.JWTToken,
				RequestTimeout:      sensorConfig.RequestTimeout,
			},
		},
	)
	if err != nil {
		klog.V(2).ErrorS(err, "Error when try to create executor")
		return nil, err
	}
	sensor := New(&SensorOpts{
		KubeConfig: kubeConfig,
		SensorName: sensorConfig.SensorName,
		EventQueueOpts: eventqueue.EventQueueOpts{
			WorkerCount:  sensorConfig.WorkerCount,
			MaxTryCount:  sensorConfig.MaxTryCount,
			RequeueDelay: sensorConfig.RequeueDelay,
		},
	}, executor)
	return &SensorRuntime{
		sensor:        sensor,
		ruleCollector: ruleCollector,
		cancelFunc:    nil,
	}, nil
}

// StartSensorRuntime will start the sensor and rule collectors.
// Rules will be automatically reloaded if any rules changes were detected.
// StartSensor will collect initial rules for the sensor.
// StartSensor will block until the sensor runtime is stopped using
// StopSensor method.
func (sr *SensorRuntime) StartSensorRuntime() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	sr.cancelFunc = cancelFunc
	sensorRules, err := sr.ruleCollector.Collect(ctx)
	if err != nil {
		klog.V(2).ErrorS(err, "Error when try to collect rules")
		return err
	}
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		wg.Done()
		sr.sensor.StartSensorAndWorkerPool(sensorRules)
	}()
	go func() {
		wg.Done()
		sr.ruleCollector.StartRuleCollector(ctx, sr.sensor)
	}()
	wg.Wait()
	return nil
}

// StopSensorRuntime stops sensor and rule collectors gracefully.
// StopSensor will drain the Queue to make sure collected events
// are processed and then it will stop the workers.
func (sr *SensorRuntime) StopSensorRuntime() {
	sr.cancelFunc()
	sr.sensor.Stop()
}

// StopOnSignal is a helper around StopSensor method to stop
// the sensor and related listeners on SIGINT or SIGTERM signals.
// Utilizes the StopSensor method which will stop all components
// gracefully.
func (sr *SensorRuntime) StopOnSignal() {
	klog.V(1).Info("Listening for SIGINT/SIGTERM")
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	klog.V(1).Info("Received an interrupt, stopping sensor")
	sr.StopSensorRuntime()
	klog.V(1).Info("Sensor stopped")
}

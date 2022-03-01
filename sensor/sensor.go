package sensor

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/config"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner/client"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/ruleinformers"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules/collector"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/utils"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

//InvalidSensorStateError is an error that is returned when the sensor state is not valid.
type InvalidSensorStateError struct {
	state         State
	requiredAnyOf []State
}

// Error implements error interface.
func (isse *InvalidSensorStateError) Error() string {
	return fmt.Sprintf("Invalid sensor state: %v, required any of: %v", isse.state, isse.requiredAnyOf)
}

// State type depicts the state of the sensor.
type State int

const (
	// INIT is the initial state of the sensor.
	INIT State = iota
	//STARTING is the state when the sensor is starting.
	STARTING
	//RUNNING is the state when the sensor is running.
	RUNNING
	//STOPPING is the state when the sensor is stopping.
	STOPPING
	//STOPPED is the state when the sensor is stopped.
	STOPPED
)

// String returns the string representation of the sensor state.
func (s State) String() string {
	return [...]string{"INIT", "STARTING", "RUNNING", "STOPPING", "STOPPED"}[s]
}

// Opts holds options related to sensor configuration
// - KubConfig : kubernetes config
// - EventQueueOpts : event queue options. Refer eventqueue.EventQueueOpts
// - SensorName: name of the sensor
type Opts struct {
	eventqueueOpts eventqueue.Opts
	KubeConfig     *rest.Config
	SensorName     string
}

// Sensor struct implements kubernetes informers to sense changes
// according to the rules defined.
// Responsible for managing all informers and event queue
// TODO: Add support for leader elect between a group of sensors with
//       the same name.
type Sensor struct {
	*Opts
	dynamicClientSet    dynamic.Interface
	ruleInformerFactory *ruleinformers.RuleInformerFactory
	queue               *eventqueue.EventQueue
	ruleInformers       map[rules.RuleID]*ruleinformers.RuleInformer
	stopChan            chan struct{}
	state               State
	sensorLock          sync.RWMutex
}

// New creates a new default Sensor. Refer Sensor struct documentation for
// more information.
// SensorOpts is required.
func New(sensorOpts *Opts, executor eventqueue.QueueExecutor) *Sensor {
	if sensorOpts == nil {
		panic("SensorOpts cannot be nil")
	}
	dynamicClientSet := dynamic.NewForConfigOrDie(sensorOpts.KubeConfig)
	queue := eventqueue.New(executor, sensorOpts.eventqueueOpts)
	return &Sensor{
		Opts:             sensorOpts,
		dynamicClientSet: dynamicClientSet,
		ruleInformerFactory: ruleinformers.NewRuleInformerFactory(
			dynamicClientSet,
			sensorOpts.SensorName,
			queue,
		),
		ruleInformers: make(map[rules.RuleID]*ruleinformers.RuleInformer),
		stopChan:      make(chan struct{}),
		state:         INIT,
		queue:         queue,
	}
}

// GetSensorState returns the current state of the sensor.
func (s *Sensor) GetSensorState() State {
	s.sensorLock.RLock()
	defer s.sensorLock.RUnlock()
	return s.state
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
	s.sensorLock.Lock()
	defer s.sensorLock.Unlock()
	klog.V(1).Infof("Reloading rules for sensor %v", s.SensorName)
	if s.state != RUNNING {
		klog.V(1).Info("Sensor is not running, skipping reloading rules")
		return
	}
	for newRuleID, newRule := range sensorRules {
		if oldRuleInformer, ok := s.ruleInformers[newRuleID]; !ok {
			klog.V(2).Infof("Rule %v is not present in the old rules, adding it", newRuleID)
			ruleInf := s.ruleInformerFactory.CreateRuleInformer(newRule)
			s.ruleInformers[newRuleID] = ruleInf
			ruleInf.Start()
		} else {
			if !reflect.DeepEqual(oldRuleInformer.Rule, newRule) {
				klog.V(2).Infof("Rule %v is present in the old rules, but the configuration is different, updating it", newRuleID)
				oldRuleInformer.Stop()
				ruleInf := s.ruleInformerFactory.CreateRuleInformer(newRule)
				s.ruleInformers[newRuleID] = ruleInf
				ruleInf.Start()
			} else {
				klog.V(2).Infof("Rule %s is not changed, skipping reloading", newRuleID)
			}
		}
	}
	for oldRuleID, oldRuleInformer := range s.ruleInformers {
		if _, ok := sensorRules[oldRuleID]; !ok {
			klog.V(2).Infof("Rule %v is not present in the new rules, removing it", oldRuleID)
			oldRuleInformer.Stop()
			delete(s.ruleInformers, oldRuleID)
		}
	}
}

// Start starts the sensor. It will start all informers which will register event handlers
// and filters based on the rules.
// Start assumes rules are valid and unique.
// Start is a blocking call, it will block until the sensor is stopped.
func (s *Sensor) Start(sensorRules map[rules.RuleID]*rules.Rule) error {
	s.sensorLock.Lock()
	if s.state != INIT && s.state != STOPPED {
		err := &InvalidSensorStateError{s.state, []State{INIT, STOPPED}}
		klog.V(1).ErrorS(err, "Unable to start sensor")
		s.sensorLock.Unlock()
		return err
	}
	klog.V(1).Info("Starting sensor")
	s.state = STARTING

	for ruleID, rule := range sensorRules {
		ruleInformer := s.ruleInformerFactory.CreateRuleInformer(rule)
		ruleInformer.Start()
		s.ruleInformers[ruleID] = ruleInformer
	}
	s.state = RUNNING
	s.sensorLock.Unlock()
	<-s.stopChan
	return nil
}

// StartSensorAndWorkerPool will start the sensor and the worker pool.
// Worker pool which is part of the eventqueue module will consume events from the queue.
func (s *Sensor) StartSensorAndWorkerPool(sensorRules map[rules.RuleID]*rules.Rule) {
	go s.Start(sensorRules)
	go s.queue.StartQueueWorkerPool()
	<-s.stopChan
}

// Stop stops the sensor. It will stop all informers which will unregister all
// event handlers.
// Stop will block until all informers are stopped.
func (s *Sensor) Stop() error {
	s.sensorLock.Lock()
	defer s.sensorLock.Unlock()
	if s.state != RUNNING {
		err := &InvalidSensorStateError{s.state, []State{RUNNING}}
		klog.V(1).ErrorS(err, "Unable to stop sensor")
		return err
	}
	s.state = STOPPING
	klog.V(1).Info("Stopping sensor")
	for _, ruleInf := range s.ruleInformers {
		ruleInf.Stop()
	}
	close(s.stopChan)
	klog.V(1).Info("Stopped all informers, draining queue")
	s.queue.ShutDownWithDrain()
	s.state = STOPPED
	return nil
}

// Runtime sets up the sensor runtime and manages it.
// TODO: Rework cancelFunc in rule collectors
type Runtime struct {
	sensor        *Sensor
	ruleCollector *collector.ConfigMapRuleCollector
	cancelFunc    context.CancelFunc
}

// SetupNewSensorRuntime will setup the sensor and return a sensor runtime.
// SetupSensor will collect the required KubeConfig and initialize the sensor
// to be able to start.
func SetupNewSensorRuntime(sensorConfig *config.Config) (*Runtime, error) {
	kubeConfig, err := utils.GetKubeAPIConfig(sensorConfig.KubeConfigPath)
	if err != nil {
		klog.V(2).ErrorS(err, "Error when trying to create kube config")
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		klog.V(2).ErrorS(err, "Error when trying to create kube client")
		return nil, err
	}
	ruleCollector := collector.NewConfigMapRuleCollector(kubeClient, sensorConfig.SensorNamespace, sensorConfig.SensorRuleConfigMapLabel)
	executor, err := executor.New(
		executor.Type(sensorConfig.ExecutorType),
		executor.Opts{
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
	sensor := New(&Opts{
		KubeConfig: kubeConfig,
		SensorName: sensorConfig.SensorName,
		eventqueueOpts: eventqueue.Opts{
			WorkerCount:  sensorConfig.WorkerCount,
			MaxTryCount:  sensorConfig.MaxTryCount,
			RequeueDelay: sensorConfig.RequeueDelay,
		},
	}, executor)
	return &Runtime{
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
func (sr *Runtime) StartSensorRuntime() error {
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
func (sr *Runtime) StopSensorRuntime() {
	sr.cancelFunc()
	sr.sensor.Stop()
}

// StopOnSignal is a helper around StopSensor method to stop
// the sensor and related listeners on SIGINT or SIGTERM signals.
// Utilizes the StopSensor method which will stop all components
// gracefully.
func (sr *Runtime) StopOnSignal() {
	klog.V(1).Info("Listening for SIGINT/SIGTERM")
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	klog.V(1).Info("Received an interrupt, stopping sensor")
	sr.StopSensorRuntime()
	klog.V(1).Info("Sensor stopped")
}

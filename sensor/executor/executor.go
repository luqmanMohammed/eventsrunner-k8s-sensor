package executor

import (
	"fmt"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner/client"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/script"
	"k8s.io/klog/v2"
)

// Executor interface should be implemented by all executors.
// Executor interface is compatible with the QueueExecutor interface in eventqueue package.
type Executor interface {
	Execute(event *eventqueue.Event) error
}

const (
	SCRIPT ExecutorType = "script"
	ER     ExecutorType = "eventsrunner"
	LOG    ExecutorType = "log"
)

// ExecutorType is the type of the executor.
type ExecutorType string

// ExecutorOpts contains options for creating any type of executor.
// New method will error out if required config is not provided for
// the specific executor
type ExecutorOpts struct {
	ScriptDir    string
	ScriptPrefix string
	AuthType     client.AuthType
	client.EventsRunnerClientOpts
}

// New creates a new instance of the executor and returns it.
// It will error out if the executor type is not supported.
// Supported executor types are:
// - script
// - eventsrunner
// - log
// If the required config is not provided for the specific executor,
// New method will error out.
func New(exType ExecutorType, exOpts ExecutorOpts) (Executor, error) {
	switch exType {
	case SCRIPT:
		return script.New(exOpts.ScriptDir, exOpts.ScriptPrefix)
	case ER:
		return eventsrunner.New(exOpts.AuthType, &exOpts.EventsRunnerClientOpts)
	case LOG:
		return &LogExecutor{}, nil
	}
	return nil, fmt.Errorf("invalid executor type: %s", exType)
}

// LogExecutor is the simplest executor which will just log the event.
// It is used for testing purposes.
// Compatible with the Executor Interface
type LogExecutor struct{}

func (le *LogExecutor) Execute(event *eventqueue.Event) error {
	klog.V(3).Infof("Executing log executor for rule: %s", event.RuleID)
	return nil
}

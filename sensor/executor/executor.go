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
	// SCRIPT is the type of the script executor.
	SCRIPT Type = "script"
	// ER is the type of the eventsrunner executor.
	ER Type = "eventsrunner"
	// LOG is the type of the log executor.
	LOG Type = "log"
)

// Type is the type of the executor.
type Type string

// Opts contains options for creating any type of executor.
// New method will error out if a required config is not provided for
// the specific executor.
// - ScriptDir: Directory where the scripts are located.
// - ScriptPrefix: Prefix of the scripts.
// - AuthType: Type of authentication to be used for eventsrunner client auth.
// - EventsRunnerClientOpts: Options for creating eventsrunner client. Refer to
// the eventsrunner client package for more details.
type Opts struct {
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
// If the a required config is not provided for the specific executor,
// New method will error out.
func New(exType Type, exOpts Opts) (Executor, error) {
	klog.V(1).Infof("Creating new executor of type: %s", exType)
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

// Execute logs the event.
func (le *LogExecutor) Execute(event *eventqueue.Event) error {
	klog.V(3).Infof("Executing log executor for rule: %s", event.RuleID)
	return nil
}

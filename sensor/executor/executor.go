package executor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/erclient"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"k8s.io/klog/v2"
)

var (
	// ErrInvalidScriptFile is returned when the script file is not a regular file.
	ErrInvalidScriptFile = errors.New("invalid script file")
	// ErrFileIsNotExecutable is returned when the script file is not executable.
	ErrFileIsNotExecutable = errors.New("file is not executable")
)

// ScriptExecutor is an implementation of QueueExecutor
// interface in the eventqueue package.
// which is used to execute a script.
// Scripts should be in the following naming convention
// <ScriptDir>/<ScriptPrefix>-<RuleID>.sh .
// Scripts should be executable.
// CAUTION: Make sure to vet the script before allowing the sensor to
// run it, since the sensor does not do any kind of verification.
// Implemented as a Proof of Concept. Passing on the event to eventsrunner
// will be more scalable, easier to maintain and secure.
type ScriptExecutor struct {
	scriptDir    string
	scriptPrefix string
}

// NewScriptExecutor creates a new instance of ScriptExecutor.
func NewScriptExecutor(scriptDir, scriptPrefix string) *ScriptExecutor {
	return &ScriptExecutor{
		scriptDir:    scriptDir,
		scriptPrefix: scriptPrefix,
	}
}

// Execute executes the script for the given event.
// Execute function will construct the script path and execute it.
// The relevent event information will be passed to the script as
// an base64 encoded JSON string in the form of an environment variable
// with the name EVENT.
// Execute will return errors if the script is not an executable or if
// the script is invalid.
// OS STDOUT and STDERR will be used for the script.
// TODO: Add file STDOUT and STDERR for scripts
func (se *ScriptExecutor) Execute(event *eventqueue.Event) error {
	script := fmt.Sprintf("%s/%s-%s.sh", se.scriptDir, se.scriptPrefix, event.RuleID)
	klog.V(3).Infof("Executing script %s", script)
	if fileInfo, err := os.Stat(script); err != nil {
		return err
	} else if fileInfo.IsDir() {
		return ErrInvalidScriptFile
	} else if fileInfo.Mode()&0111 == 0 {
		return ErrFileIsNotExecutable
	}
	eventJson, err := json.Marshal(event)
	if err != nil {
		return err
	}
	encodedEventJson := base64.StdEncoding.EncodeToString(eventJson)
	cmd := exec.Command(script)
	env := os.Environ()
	env = append(env, fmt.Sprintf("EVENT=%s", encodedEventJson))
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// EventsRunnerExecutor is an implementation of QueueExecutor interface
// in the eventqueue package.
// It utilizes the eventsrunner client to send events to the eventsrunner.
type EventsRunnerExecutor struct {
	eventsRunnerClient *erclient.EventsRunnerClient
}

// NewEventsRunnerExecutor creates a new instance of EventsRunnerExecutor.
// authType determines the type of authentication to be used by the client
// to communicate with the server.
func NewEventsRunnerExecutor(authType erclient.AuthType, eventsRunnerOpts *erclient.EventsRunnerClientOpts) (*EventsRunnerExecutor, error) {
	eventsRunnerClient, err := erclient.New(authType, eventsRunnerOpts)
	if err != nil {
		return nil, err
	}
	return &EventsRunnerExecutor{
		eventsRunnerClient: eventsRunnerClient,
	}, nil
}

// Execute sends the event to the eventsrunner.
func (ere EventsRunnerExecutor) Execute(event *eventqueue.Event) error {
	return ere.eventsRunnerClient.ProcessEvent(event)
}

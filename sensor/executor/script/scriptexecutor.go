package script

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/config"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"k8s.io/klog/v2"
)

var (
	// ErrInvalidScriptFile is returned when the script file is not a regular file.
	ErrInvalidScriptFile = errors.New("invalid script file")
	// ErrFileIsNotExecutable is returned when the script file is not executable.
	ErrFileIsNotExecutable = errors.New("file is not executable")
)

// Executor is an implementation of QueueExecutor interface in the
// eventqueue package.
// Script executor will execute configured scripts according to the event
// received from the event queue.
// Scripts should be in the following naming convention
// <ScriptDir>/<ScriptPrefix>-<RuleID>.sh .
// Scripts should be executable.
// CAUTION: Make sure to vet the script before allowing the sensor to
// run it, since the sensor does not do any kind of verification.
// Implemented as a Proof of Concept. Passing on the event to eventsrunner
// will be more scalable, easier to maintain and secure.
type Executor struct {
	scriptDir    string
	scriptPrefix string
}

// New creates a new instance of script based Executor.
// ScriptDir and ScriptPrefix are configs are required.
func New(scriptDir, scriptPrefix string) (*Executor, error) {
	if err := config.AnyRequestedConfigMissing(map[string]interface{}{
		"ScriptDir":    scriptDir,
		"ScriptPrefix": scriptPrefix,
	}); err != nil {
		return nil, err
	}
	return &Executor{
		scriptDir:    scriptDir,
		scriptPrefix: scriptPrefix,
	}, nil
}

// Execute executes the script for the given event.
// Execute function will construct the script path and execute it.
// The relevant event information will be passed to the script as
// an base64 encoded JSON string in the form of an environment variable
// with the name EVENT.
// Execute will return errors if the script is not an executable or if
// the script is invalid.
// OS STDOUT and STDERR will be used for the script.
// TODO: Add file STDOUT and STDERR for scripts
func (se *Executor) Execute(event *eventqueue.Event) error {
	script := fmt.Sprintf("%s/%s-%s.sh", se.scriptDir, se.scriptPrefix, event.RuleID)
	klog.V(2).Infof("Executing script %s for ruleID %s", script, event.RuleID)
	if fileInfo, err := os.Stat(script); err != nil {
		return err
	} else if fileInfo.IsDir() {
		return ErrInvalidScriptFile
	} else if fileInfo.Mode()&0111 == 0 {
		return ErrFileIsNotExecutable
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	encodedEventJSON := base64.StdEncoding.EncodeToString(eventJSON)
	cmd := exec.Command(script)
	env := os.Environ()
	env = append(env, fmt.Sprintf("EVENT=%s", encodedEventJSON))
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		klog.V(2).ErrorS(err, fmt.Sprintf("Failed to execute script %s for rule %s", script, event.RuleID))
		return err
	}
	klog.V(2).Infof("Successfully executed script %s for rule %s", script, event.RuleID)
	return nil
}

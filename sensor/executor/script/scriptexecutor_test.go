package script

import (
	"os"
	"testing"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	testEvent = &eventqueue.Event{
		EventType: rules.ADDED,
		RuleID:    "test-rule",
		Objects: []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
					},
				},
			},
		},
	}

	testScript = `#!/bin/bash

# Check if EVENT environment variable is set
if [ -z "$EVENT" ]; then
	echo "EVENT is not set"
	exit 1
fi

# Try to base64 decode EVENT variable
EVENT_DECODED=$(echo "$EVENT" | base64 -d)
EVENT_DECODED_RULE_ID=$(echo "$EVENT_DECODED" | jq -r '.ruleID')
# Check if EVENT_DECODED_RULE_ID is test-rule
if [ "$EVENT_DECODED_RULE_ID" != "test-rule" ]; then
	echo "Rule ID is not test-rule"
	exit 1
fi

exit 0
`
)

func TestScriptExecutorErrorHandling(t *testing.T) {
	t.Run("should return error if required params are not provided", func(t *testing.T) {
		_, err := New("", "script")
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		t.Log(err)
	})
	t.Run("should return error if script is not present", func(t *testing.T) {
		se, _ := New("/tmp", "script")
		event := testEvent
		err := se.Execute(event)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})
	t.Run("should return error if script is not valid", func(t *testing.T) {
		if err := os.Mkdir("/tmp/script-test-rule.sh", 0777); err != nil {
			t.Fatalf("Failed to setup test: %v", err)
		}
		defer func() {
			if err := os.RemoveAll("/tmp/script-test-rule.sh"); err != nil {
				t.Fatalf("Failed to delete test setup: %v", err)
			}
		}()
		se, _ := New("/tmp", "script")
		event := testEvent
		err := se.Execute(event)
		if err == nil {
			t.Errorf("expected error, got nil")
		} else {
			if err != ErrInvalidScriptFile {
				t.Errorf("expected ErrInvalidScriptFile, got %v", err)
			}
		}
	})
	t.Run("should return error if script is not executable", func(t *testing.T) {
		f, err := os.Create("/tmp/script-test-rule.sh")
		if err != nil {
			t.Fatalf("Failed to create test script: %v", err)
		}
		if _, err = f.Write([]byte(testScript)); err != nil {
			t.Fatalf("Failed to write test script: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Failed to close test script: %v", err)
		}
		defer func() {
			if err := os.Remove("/tmp/script-test-rule.sh"); err != nil {
				t.Fatalf("Failed to delete test script: %v", err)
			}
		}()
		se, _ := New("/tmp", "script")
		event := testEvent
		err = se.Execute(event)
		if err == nil {
			t.Errorf("expected error, got nil")
		} else {
			if err != ErrFileIsNotExecutable {
				t.Errorf("expected ErrFileIsNotExecutable, got %v", err)
			}
		}
	})
	t.Run("should return error if script exists with non zero", func(t *testing.T) {
		testFailingScript := `#!/bin/bash
exit 1`
		f, err := os.Create("/tmp/script-test-rule.sh")
		if err != nil {
			t.Fatalf("Failed to create script due to %v", err)
		}
		if err = f.Chmod(0777); err != nil {
			t.Fatalf("Failed to chmod script due to %v", err)
		}
		if _, err = f.Write([]byte(testFailingScript)); err != nil {
			t.Fatalf("Failed to write script due to %v", err)
		}
		if err = f.Close(); err != nil {
			t.Fatalf("Failed to close script due to %v", err)
		}
		defer func() {
			if err := os.Remove("/tmp/script-test-rule.sh"); err != nil {
				t.Fatalf("Failed to delete script due to %v", err)
			}
		}()
		se, _ := New("/tmp", "script")
		event := testEvent
		err = se.Execute(event)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		t.Log(err)
	})
}

func TestScriptExecutor(t *testing.T) {
	// Create test script at /tmp/script-test-rule.sh
	f, err := os.Create("/tmp/script-test-rule.sh")
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}
	if err = f.Chmod(0777); err != nil {
		t.Fatalf("Failed to chmod test script: %v", err)
	}
	_, err = f.Write([]byte(testScript))
	if err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}
	if err = f.Close(); err != nil {
		t.Fatalf("Failed to close test script: %v", err)
	}
	defer func() {
		// Delete /tmp/script-test-rule.sh
		if err := os.Remove("/tmp/script-test-rule.sh"); err != nil {
			t.Fatalf("Failed to delete test script: %v", err)
		}
	}()

	t.Run("should execute script", func(t *testing.T) {
		se, _ := New("/tmp", "script")
		event := testEvent
		err = se.Execute(event)
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
}

package script

import (
	"os"
	"testing"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	test_event = &eventqueue.Event{
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

	test_script = `#!/bin/bash

# Check if EVENT environment variable is set
if [ -z "$EVENT" ]; then
	echo "EVENT is not set"
	exit 1
fi

# Try to base64 decode EVENT variable
EVENT_DECODED=$(echo "$EVENT" | base64 -d)
EVENT_DECODED_RULE_ID=$(echo "$EVENT_DECODED" | jq -r '.RuleID')
# Check if EVENT_DECODED_RULE_ID is test-rule
if [ "$EVENT_DECODED_RULE_ID" != "test-rule" ]; then
	echo "Rule ID is not test-rule"
	exit 1
fi

exit 0
`
)

func TestScriptExecutor(t *testing.T) {
	// Create test script at /tmp/script-test-rule.sh
	f, err := os.Create("/tmp/script-test-rule.sh")
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}
	if err = f.Chmod(0777); err != nil {
		t.Fatalf("Failed to chmod test script: %v", err)
	}
	_, err = f.Write([]byte(test_script))
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
		se := New("/tmp", "script")
		event := test_event
		err = se.Execute(event)
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
}

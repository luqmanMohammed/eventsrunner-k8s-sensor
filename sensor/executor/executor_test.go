package executor

import (
	"testing"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
)

func TestLogExecutor(t *testing.T) {
	_, err := New(Type("invalid"), Opts{})
	if err == nil {
		t.Fatalf("Expected error when creating executor of invalid type")
	}

	executor, err := New(LOG, Opts{})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	event := eventqueue.Event{
		RuleID: "test",
	}
	if err := executor.Execute(&event); err != nil {
		t.Fatalf("Failed to execute executor: %v", err)
	}
}

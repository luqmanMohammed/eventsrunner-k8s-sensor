package executor

import (
	"testing"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner/client"
)

func TestLogExecutor(t *testing.T) {
	_, err := New(Type("invalid"), Opts{})
	if err == nil {
		t.Fatalf("Expected error when creating executor of invalid type")
	}
	if _, err := New(Type("script"), Opts{
		ScriptDir:    "/tmp",
		ScriptPrefix: "script",
	}); err != nil {
		t.Fatalf("Failed to create script type executor: %v", err)
	}
	if _, err := New(Type("eventsrunner"), Opts{
		AuthType: "jwt",
		EventsRunnerClientOpts: client.EventsRunnerClientOpts{
			EventsRunnerBaseURL: "http://localhost:9090",
			JWTToken:            "test-token",
		},
	}); err != nil {
		t.Fatalf("Failed to create eventsrunner type executor: %v", err)
	}
	executor, err := New(Type("log"), Opts{})
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

package eventsrunner

import (
	"testing"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner/client"
)

func TestEventsRunner(t *testing.T) {
	if _, err := New(client.AuthType("invalid"), &client.EventsRunnerClientOpts{
		EventsRunnerBaseURL: "http://localhost:9090",
	}); err == nil {
		t.Fatalf("Expected error when creating executor with auth type of invalid type")
	}

	eventsRunnerExec, err := New(client.JWT, &client.EventsRunnerClientOpts{
		EventsRunnerBaseURL: "http://localhost:9090",
		JWTToken:            "test-token",
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	event := eventqueue.Event{
		RuleID: "test",
	}

	if err := eventsRunnerExec.Execute(&event); err == nil {
		t.Fatalf("Should have failed since server is not running")
	}
}

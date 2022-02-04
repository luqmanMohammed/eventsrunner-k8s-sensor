package eventsrunner

import (
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner/client"
)

// EventsRunnerExecutor is an implementation of QueueExecutor interface
// in the eventqueue package.
// It utilizes the eventsrunner client to send events to the eventsrunner.
type EventsRunnerExecutor struct {
	eventsRunnerClient *client.EventsRunnerClient
}

// NewEventsRunnerExecutor creates a new instance of EventsRunnerExecutor.
// authType determines the type of authentication to be used by the client
// to communicate with the server.
func New(authType client.AuthType, eventsRunnerOpts *client.EventsRunnerClientOpts) (*EventsRunnerExecutor, error) {
	eventsRunnerClient, err := client.New(authType, eventsRunnerOpts)
	if err != nil {
		return nil, err
	}
	return &EventsRunnerExecutor{
		eventsRunnerClient: eventsRunnerClient,
	}, nil
}

// Execute sends the event to the eventsrunner.
func (ere *EventsRunnerExecutor) Execute(event *eventqueue.Event) error {
	return ere.eventsRunnerClient.ProcessEvent(event)
}

package eventsrunner

import (
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/eventqueue"
	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/executor/eventsrunner/client"
)

// EventsRunnerExecutor is an implementation of QueueExecutor interface
// in the eventqueue package.
// It utilizes the eventsrunner client to send events to the eventsrunner sever
// to be processed.
type EventsRunnerExecutor struct {
	eventsRunnerClient *client.EventsRunnerClient
}

// NewEventsRunnerExecutor creates a new instance of EventsRunnerExecutor.
// authType determines the type of authentication to be used by the client
// to communicate with the server.
// Options used by eventsrunner based executor:
// - EventsRunnerBaseURL: Base URL of the eventsrunner server.
// - CaCertPath: Path to the CA certificate for mTLS and/or server verification.
// - ClientCertPath: Path to the client certificate.
// - ClientKeyPath: Path to the client key.
// - RequestTimeout: Timeout for the request.
// - JWTToken: JWT token to be used for authentication.
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

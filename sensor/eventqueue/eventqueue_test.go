package eventqueue

import (
	"errors"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	queueOpts = EventQueueOpts{
		WorkerCount:  1,
		MaxTryCount:  3,
		RequeueDelay: 100 * time.Millisecond,
	}
	basicEvents = []*Event{
		{
			EventType: "added",
			RuleID:    "rule1",
			Objects:   []*unstructured.Unstructured{},
		},
		{
			EventType: "deleted",
			RuleID:    "rule2",
			Objects:   []*unstructured.Unstructured{},
		},
		{
			EventType: "modified",
			RuleID:    "rule3",
			Objects:   []*unstructured.Unstructured{},
		},
	}
	retryTest = []*Event{
		{
			EventType: "added",
			RuleID:    "fail-rule1",
			Objects:   []*unstructured.Unstructured{},
		},
	}
)

type mockExecutor struct {
	tries  int
	events []*Event
}

func (me *mockExecutor) Execute(event *Event) error {
	if strings.HasPrefix(string(event.RuleID), "fail") {
		me.tries++
		return errors.New("failed to execute")
	}
	me.events = append(me.events, event)
	return nil
}

func TestQueueWorkerNormalBehavior(t *testing.T) {
	mockExec := &mockExecutor{
		tries:  0,
		events: []*Event{},
	}
	queue := New(mockExec, queueOpts)
	defer queue.ShutDownWithDrain()
	go queue.StartQueueWorkerPool()
	for _, event := range basicEvents {
		queue.Add(event)
	}
	retryCount := 0
	for {
		if retryCount >= 5 {
			t.Fatalf("Waited 5 seconds for retry, but still not all items were processed")
		}
		if len(mockExec.events) == len(basicEvents) {
			break
		}
		time.Sleep(1 * time.Second)
		retryCount++
	}
}

func TestQueueWorkerRetry(t *testing.T) {
	mockExec := &mockExecutor{
		tries:  0,
		events: []*Event{},
	}
	queue := New(mockExec, queueOpts)
	defer queue.ShutDownWithDrain()
	for _, event := range retryTest {
		queue.Add(event)
	}
	go queue.StartQueueWorkerPool()
	retryCount := 0
	for {
		if retryCount >= 5 {
			t.Fatalf("Waited 5 seconds. Expected tries to be 3 but was %d", mockExec.tries)
		}
		if mockExec.tries == queue.maxTryCount {
			break
		}
		time.Sleep(1 * time.Second)
		retryCount++
	}
}

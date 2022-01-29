package eventqueue

import (
	"sync"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/workqueue"
)

// Event struct holds all information related to an event.
type Event struct {
	EventType rules.EventType
	RuleID    rules.RuleID
	Objects   []*unstructured.Unstructured
	tries     int `json:"-"`
}

// QueueItemExecutor all queue executors must implement this interface
type QueueExecutor interface {
	Execute(event *Event) error
}

// EventQueue wraps around the workqueue.DelayingInterface and provides
// additional functionality.
// Adds functionality to process and retry items if failed.
type EventQueue struct {
	workqueue.DelayingInterface
	workerCount  int
	maxTryCount  int
	requeueDelay time.Duration
	executor     QueueExecutor
}

// Creates a New EventQueue and returns the pointer to it.
func New(executor QueueExecutor, workerCount int, maxTryCount int, requeueDelay time.Duration) *EventQueue {
	eq := &EventQueue{
		DelayingInterface: workqueue.NewDelayingQueue(),
		workerCount:       workerCount,
		maxTryCount:       maxTryCount,
		requeueDelay:      requeueDelay,
		executor:          executor,
	}
	return eq
}

// StartQueueWorkerPool will start a pool of workers to process queue items.
// Blocks until all workers have finished. The number of workers is determined by the
// workerCount parameter.
// Closing the queue will cause the queue to be drained and all workers to exit.
func (eq *EventQueue) StartQueueWorkerPool() {
	wg := sync.WaitGroup{}
	wg.Add(eq.workerCount)
	for i := 0; i < eq.workerCount; i++ {
		go eq.startWorker(&wg)
	}
	wg.Wait()
}

// processItem will process the item from the queue.
// Execute method will be called for each item in the queue.
// If the Execute method returns an error, the item will be added to the queue
// to be retried, if the number of reties exceed the configured
// value, the item wont be retried again.
// If the Execute method returns nil, the item will be removed from the queue.
func (eq *EventQueue) processItem(event *Event) {
	event.tries++
	err := eq.executor.Execute(event)
	if err != nil {
		if event.tries < eq.maxTryCount {
			eq.DelayingInterface.AddAfter(event, eq.requeueDelay)
		}
	}
}

// startWorker will start the worker.
// An infinite loop will be started and the worker will process items, until the
// queue is closed, which will cause the worker to exit.
// If the item in the queue is not an Event, it will be ignored.
func (eq *EventQueue) startWorker(wg *sync.WaitGroup) {
	for {
		item, quit := eq.DelayingInterface.Get()
		if quit {
			break
		}
		event, ok := item.(*Event)
		if ok {
			eq.processItem(event)
		}
		eq.DelayingInterface.Done(item)
	}
	wg.Done()
}

package eventqueue

import (
	"sync"
	"time"

	"github.com/luqmanMohammed/eventsrunner-k8s-sensor/sensor/rules"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// Event struct holds all information related to an event.
// TODO: Add json tags to the struct.
type Event struct {
	EventType rules.EventType
	RuleID    rules.RuleID
	Objects   []*unstructured.Unstructured
	tries     int `json:"-"`
}

// QueueItemExecutor all queue executors must implement this interface.
// Execute method will be called for each item in the queue.
type QueueExecutor interface {
	Execute(event *Event) error
}

// MockQueueExecutor is a mock implementation of QueueExecutor interface.
// It is used for testing or as a placeholder.
type MockQueueExecutor struct{}

func (*MockQueueExecutor) Execute(event *Event) error {
	return nil
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

// EventQueueOpts holds the configuration options for the EventQueue.
// WorkerCount is the number of workers to start.
// MaxTryCount is the maximum number of times an item will be retried.
// RequeueDelay is the delay in seconds before an item is retried.
type EventQueueOpts struct {
	WorkerCount  int
	MaxTryCount  int
	RequeueDelay time.Duration
}

// Creates a New EventQueue and returns the pointer to it.
func New(executor QueueExecutor, queueOpts EventQueueOpts) *EventQueue {
	eq := &EventQueue{
		DelayingInterface: workqueue.NewDelayingQueue(),
		workerCount:       queueOpts.WorkerCount,
		maxTryCount:       queueOpts.MaxTryCount,
		requeueDelay:      queueOpts.RequeueDelay,
		executor:          executor,
	}

	return eq
}

// StartQueueWorkerPool will start a pool of workers to process queue items.
// Blocks until all workers have finished. The number of workers is determined by the
// workerCount parameter.
// Closing the queue will cause the queue to be drained and all workers to exit.
func (eq *EventQueue) StartQueueWorkerPool() {
	klog.V(2).Info("Starting queue worker pool")
	klog.V(2).Infof("Number of workers: %d", eq.workerCount)
	wg := sync.WaitGroup{}
	wg.Add(eq.workerCount)
	for i := 0; i < eq.workerCount; i++ {
		go eq.startWorker(&wg)
	}
	wg.Wait()
	klog.V(2).Info("All queue workers have successfully stopped")
}

// processItem will process the item from the queue.
// Execute method will be called for each item in the queue.
// If the Execute method returns an error, the item will be added back into the
// queue after the configured delay.
// Event will be retried up to the configured maxTryCount, then it will removed from the
// the queue.
// If the Execute method returns nil for the error, the item will be removed from the queue.
func (eq *EventQueue) processItem(event *Event) {
	event.tries++
	klog.V(3).Infof("Processing event from rule %s, tries: %d", event.RuleID, event.tries)
	err := eq.executor.Execute(event)
	if err != nil {
		klog.V(2).ErrorS(err, "Error during execution of event")
		if event.tries < eq.maxTryCount {
			klog.V(3).Info("Adding event to queue to be retried")
			eq.DelayingInterface.AddAfter(event, eq.requeueDelay)
		}
	}
}

// startWorker will start the worker.
// An infinite loop will be started and the worker will process items, until the
// queue is closed, which will cause the worker to exit.
// If the item in the queue is not an Event, it will be ignored.
func (eq *EventQueue) startWorker(wg *sync.WaitGroup) {
	klog.V(3).Info("Starting worker")
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
	klog.V(3).Info("Worker has successfully stopped")
	wg.Done()
}

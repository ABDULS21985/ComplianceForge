package queue

import (
	"reflect"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestTopologyForCreatesBoundedRetryAndTerminalQueues(t *testing.T) {
	config := DefaultConfig("amqp://localhost:5672/")
	topology, err := TopologyFor("complianceforge.worker", config)
	if err != nil {
		t.Fatal(err)
	}
	if topology.RetryQueue != "complianceforge.worker.retry" || topology.DeadLetterQueue != "complianceforge.worker.dead" || topology.QuarantineQueue != "complianceforge.worker.quarantine" {
		t.Fatalf("unexpected queue names: %+v", topology)
	}
	if topology.PrimaryArguments["x-queue-type"] != "quorum" || topology.PrimaryArguments["x-delivery-limit"] != int32(config.MaxAttempts+2) {
		t.Fatalf("primary durability arguments missing: %+v", topology.PrimaryArguments)
	}
	if topology.PrimaryArguments["x-dead-letter-exchange"] != config.DeadLetterExchange {
		t.Fatalf("primary DLX missing: %+v", topology.PrimaryArguments)
	}
	if topology.RetryArguments["x-message-ttl"] != config.RetryDelay.Milliseconds() || topology.RetryArguments["x-dead-letter-exchange"] != config.Exchange {
		t.Fatalf("retry arguments missing: %+v", topology.RetryArguments)
	}
}

func TestClassicTopologyOmitsQuorumDeliveryLimit(t *testing.T) {
	config := DefaultConfig("amqp://localhost:5672/")
	config.QueueType = "classic"
	topology, err := TopologyFor("worker", config)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := topology.PrimaryArguments["x-delivery-limit"]; exists {
		t.Fatalf("classic queue received quorum-only delivery limit: %+v", topology.PrimaryArguments)
	}
}

type exchangeDeclaration struct {
	name, kind string
	durable    bool
}

type queueDeclaration struct {
	name, queueType string
	durable         bool
}

type queueBinding struct {
	name, key, exchange string
}

type topologyRecorder struct {
	exchanges []exchangeDeclaration
	queues    []queueDeclaration
	bindings  []queueBinding
}

func (r *topologyRecorder) ExchangeDeclare(name, kind string, durable, _, _, _ bool, _ amqp.Table) error {
	r.exchanges = append(r.exchanges, exchangeDeclaration{name: name, kind: kind, durable: durable})
	return nil
}

func (r *topologyRecorder) QueueDeclare(name string, durable, _, _, _ bool, args amqp.Table) (amqp.Queue, error) {
	r.queues = append(r.queues, queueDeclaration{name: name, durable: durable, queueType: args["x-queue-type"].(string)})
	return amqp.Queue{Name: name}, nil
}

func (r *topologyRecorder) QueueBind(name, key, exchange string, _ bool, _ amqp.Table) error {
	r.bindings = append(r.bindings, queueBinding{name: name, key: key, exchange: exchange})
	return nil
}

func TestDeclareTopologyUsesDurableDirectObjects(t *testing.T) {
	config := DefaultConfig("amqp://localhost:5672/")
	topology, err := TopologyFor("worker", config)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &topologyRecorder{}
	if err := declareTopology(recorder, config, topology); err != nil {
		t.Fatal(err)
	}
	if len(recorder.exchanges) != 4 || len(recorder.queues) != 4 || len(recorder.bindings) != 4 {
		t.Fatalf("incomplete topology: %+v", recorder)
	}
	for _, exchange := range recorder.exchanges {
		if exchange.kind != "direct" || !exchange.durable {
			t.Fatalf("exchange is not durable/direct: %+v", exchange)
		}
	}
	for _, queue := range recorder.queues {
		if !queue.durable || queue.queueType != "quorum" {
			t.Fatalf("queue is not durable/quorum: %+v", queue)
		}
	}
	wantBindings := []queueBinding{
		{name: "worker", key: "worker", exchange: config.Exchange},
		{name: "worker.retry", key: "worker", exchange: config.RetryExchange},
		{name: "worker.dead", key: "worker", exchange: config.DeadLetterExchange},
		{name: "worker.quarantine", key: "worker", exchange: config.QuarantineExchange},
	}
	if !reflect.DeepEqual(recorder.bindings, wantBindings) {
		t.Fatalf("bindings = %+v, want %+v", recorder.bindings, wantBindings)
	}
}

func TestTopologyRejectsUnsafeName(t *testing.T) {
	if _, err := TopologyFor("bad queue/name", DefaultConfig("amqp://localhost:5672/")); err == nil {
		t.Fatal("expected invalid queue name error")
	}
}

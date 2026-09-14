package queue

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Topology contains the deterministic broker objects for one logical queue.
type Topology struct {
	Queue             string
	RetryQueue        string
	DeadLetterQueue   string
	QuarantineQueue   string
	RoutingKey        string
	PrimaryArguments  amqp.Table
	RetryArguments    amqp.Table
	TerminalArguments amqp.Table
}

func TopologyFor(queueName string, cfg Config) (Topology, error) {
	// Reserve room for the longest suffix.
	if err := validateBrokerName(queueName, 240); err != nil {
		return Topology{}, fmt.Errorf("invalid queue name: %w", err)
	}
	baseArguments := amqp.Table{"x-queue-type": cfg.QueueType}
	primaryArguments := cloneTable(baseArguments)
	primaryArguments["x-dead-letter-exchange"] = cfg.DeadLetterExchange
	primaryArguments["x-dead-letter-routing-key"] = queueName
	if cfg.QueueType == "quorum" {
		primaryArguments["x-delivery-limit"] = int32(cfg.MaxAttempts + 2)
	}

	retryArguments := cloneTable(baseArguments)
	retryArguments["x-message-ttl"] = cfg.RetryDelay.Milliseconds()
	retryArguments["x-dead-letter-exchange"] = cfg.Exchange
	retryArguments["x-dead-letter-routing-key"] = queueName

	return Topology{
		Queue:             queueName,
		RetryQueue:        queueName + ".retry",
		DeadLetterQueue:   queueName + ".dead",
		QuarantineQueue:   queueName + ".quarantine",
		RoutingKey:        queueName,
		PrimaryArguments:  primaryArguments,
		RetryArguments:    retryArguments,
		TerminalArguments: cloneTable(baseArguments),
	}, nil
}

type topologyChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
}

func declareTopology(channel topologyChannel, cfg Config, topology Topology) error {
	for _, exchange := range []string{cfg.Exchange, cfg.RetryExchange, cfg.DeadLetterExchange, cfg.QuarantineExchange} {
		if err := channel.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", exchange, err)
		}
	}

	declarations := []struct {
		name string
		args amqp.Table
	}{
		{topology.Queue, topology.PrimaryArguments},
		{topology.RetryQueue, topology.RetryArguments},
		{topology.DeadLetterQueue, topology.TerminalArguments},
		{topology.QuarantineQueue, topology.TerminalArguments},
	}
	for _, declaration := range declarations {
		if _, err := channel.QueueDeclare(declaration.name, true, false, false, false, declaration.args); err != nil {
			return fmt.Errorf("declare queue %s: %w", declaration.name, err)
		}
	}

	bindings := []struct {
		name, key, exchange string
	}{
		{topology.Queue, topology.RoutingKey, cfg.Exchange},
		{topology.RetryQueue, topology.RoutingKey, cfg.RetryExchange},
		{topology.DeadLetterQueue, topology.RoutingKey, cfg.DeadLetterExchange},
		{topology.QuarantineQueue, topology.RoutingKey, cfg.QuarantineExchange},
	}
	for _, binding := range bindings {
		if err := channel.QueueBind(binding.name, binding.key, binding.exchange, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", binding.name, err)
		}
	}
	return nil
}

func cloneTable(source amqp.Table) amqp.Table {
	clone := make(amqp.Table, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

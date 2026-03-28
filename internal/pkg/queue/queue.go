package queue

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueService defines the interface for message queue operations.
type QueueService interface {
	// Publish sends a message to the specified queue.
	Publish(ctx context.Context, queueName string, message []byte) error

	// Subscribe registers a handler function that is called for each message
	// received on the specified queue. This call blocks until the context is
	// cancelled or an error occurs.
	Subscribe(ctx context.Context, queueName string, handler func([]byte) error) error

	// Close shuts down the connection to the message broker.
	Close() error
}

// RabbitMQService implements QueueService using RabbitMQ via amqp091-go.
type RabbitMQService struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

// NewRabbitMQService creates a new RabbitMQService connected to the given
// AMQP URL (e.g. "amqp://guest:guest@localhost:5672/").
func NewRabbitMQService(url string) (*RabbitMQService, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ at %s: %w", url, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	return &RabbitMQService{
		conn:    conn,
		channel: ch,
	}, nil
}

// Publish sends a message to the specified queue. The queue is declared
// automatically if it does not already exist.
func (r *RabbitMQService) Publish(ctx context.Context, queueName string, message []byte) error {
	_, err := r.channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", queueName, err)
	}

	err = r.channel.PublishWithContext(ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        message,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish to queue %s: %w", queueName, err)
	}

	return nil
}

// Subscribe consumes messages from the specified queue and passes each message
// body to the handler. It blocks until the context is cancelled.
func (r *RabbitMQService) Subscribe(ctx context.Context, queueName string, handler func([]byte) error) error {
	_, err := r.channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", queueName, err)
	}

	msgs, err := r.channel.Consume(
		queueName,
		"",    // consumer tag (auto-generated)
		false, // autoAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to consume from queue %s: %w", queueName, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("channel closed for queue %s", queueName)
			}
			if err := handler(msg.Body); err != nil {
				_ = msg.Nack(false, true) // requeue on failure
				continue
			}
			_ = msg.Ack(false)
		}
	}
}

// Close shuts down the channel and connection.
func (r *RabbitMQService) Close() error {
	if r.channel != nil {
		if err := r.channel.Close(); err != nil {
			return fmt.Errorf("failed to close channel: %w", err)
		}
	}
	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
	}
	return nil
}

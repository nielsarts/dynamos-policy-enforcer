package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Consumer handles RabbitMQ message consumption
type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	queue   string
	logger  *zap.Logger
}

// NewConsumer creates a new RabbitMQ consumer
func NewConsumer(amqpURL string, queue string, prefetchCount int, logger *zap.Logger) (*Consumer, error) {
	logger.Info("connecting to RabbitMQ", zap.String("url", amqpURL))

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Set QoS (prefetch count)
	if err := channel.Qos(prefetchCount, 0, false); err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to set QoS: %w", err)
	}

	// Declare queue (idempotent)
	_, err = channel.QueueDeclare(
		queue, // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	logger.Info("connected to RabbitMQ", zap.String("queue", queue))

	return &Consumer{
		conn:    conn,
		channel: channel,
		queue:   queue,
		logger:  logger,
	}, nil
}

// Consume starts consuming messages from the queue
func (c *Consumer) Consume() (<-chan amqp.Delivery, error) {
	msgs, err := c.channel.Consume(
		c.queue, // queue
		"",      // consumer tag
		false,   // auto-ack
		false,   // exclusive
		false,   // no-local
		false,   // no-wait
		nil,     // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register consumer: %w", err)
	}

	c.logger.Info("started consuming messages")
	return msgs, nil
}

// Close closes the channel and connection
func (c *Consumer) Close() error {
	c.logger.Info("closing RabbitMQ connection")

	if err := c.channel.Close(); err != nil {
		c.logger.Error("failed to close channel", zap.Error(err))
	}

	if err := c.conn.Close(); err != nil {
		c.logger.Error("failed to close connection", zap.Error(err))
		return err
	}

	c.logger.Info("RabbitMQ connection closed")
	return nil
}

// NotifyClose returns a channel that receives connection close notifications
func (c *Consumer) NotifyClose() chan *amqp.Error {
	return c.conn.NotifyClose(make(chan *amqp.Error))
}

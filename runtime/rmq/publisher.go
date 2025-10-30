package rmq

// Publisher is a generic AMQP publisher that targets a specific exchange and routing key.
// It lives in the rmq package so it can be reused by multiple domains without coupling.
type Publisher struct {
	client     *Client
	exchange   string
	routingKey string
}

// NewPublisher creates a new Publisher for the provided client, exchange and routing key.
func NewPublisher(client *Client, exchange string, routingKey string) *Publisher {
	return &Publisher{
		client:     client,
		exchange:   exchange,
		routingKey: routingKey,
	}
}

// Send publishes the provided message to this publisher's exchange and routing key.
func (p *Publisher) Send(msg Message) error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.SendTo(p.exchange, p.routingKey, msg)
}

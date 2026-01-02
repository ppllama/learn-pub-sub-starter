package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {

	data, err := json.Marshal(val)
	if err != nil {
		return err
	}

	if err := ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{ContentType: "application/json", Body: data}); err != nil {
		return err
	}
	return nil
}

func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {

	buffer := bytes.Buffer{}
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(val)
	if err != nil {
		return err
	}

	if err := ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{ContentType: "application/gob", Body: buffer.Bytes()}); err != nil {
		return err
	}
	return nil
}


type SimpleQueueType int

const(
	QueueDurable SimpleQueueType = iota
	QueueTansient
)

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType, // an enum to represent "durable" or "transient"
) (*amqp.Channel, amqp.Queue, error) {

	cha, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	var isdurable, autoDelete, exclusive bool

	switch queueType {
	case QueueDurable:
		isdurable = true
	case QueueTansient:
		autoDelete = true
		exclusive = true
	default:
		return nil, amqp.Queue{}, fmt.Errorf("invalid SimpleQueueType: %d", queueType)
	}

	amqpTable := amqp.Table{
		"x-dead-letter-exchange": "peril_dlx",
	}

	queue, err := cha.QueueDeclare(queueName, isdurable, autoDelete, exclusive, false, amqpTable)
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	if err := cha.QueueBind(queue.Name, key, exchange, false, nil); err != nil {
		return nil, amqp.Queue{}, err
	}

	return cha, queue, nil
}

type AckType int

const(
	Ack AckType = iota
	NackRequeue
	NackDiscard
)

func SubscribeJSON[T any](
    conn *amqp.Connection,
    exchange,
    queueName,
    key string,
    queueType SimpleQueueType, // an enum to represent "durable" or "transient"
    handler func(T) AckType,
) (error) {

	return subscribe(conn, exchange, queueName, key, queueType, handler, jsonUnmarshaller)
	// channel, _, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	// if err != nil {
	// 	return err
	// }

	// delivery, err := channel.Consume(queueName, "", false, false, false, false, nil)
	// if err != nil {
	// 	return err
	// }

	// go func() {
	// 	for d := range(delivery) {
	// 		var msg T
	// 		if err := json.Unmarshal(d.Body, &msg); err != nil {
	// 			fmt.Printf("could not unmarshal message: %v\n", err)
	// 			continue
	// 		}
	// 		ackType := handler(msg)

	// 		switch ackType {
	// 		case Ack:
	// 			d.Ack(false)
	// 			fmt.Println("Ack")
	// 		case NackRequeue:
	// 			d.Nack(false, true)
	// 			fmt.Println("NackReque")
	// 		case NackDiscard:
	// 			d.Nack(false, false)
	// 			fmt.Println("NackDiscard")
	// 		}
	// 	}
	// }()

	// return nil
}

func SubscribeGob[T any](
    conn *amqp.Connection,
    exchange,
    queueName,
    key string,
    queueType SimpleQueueType, // an enum to represent "durable" or "transient"
    handler func(T) AckType,
) (error) {

	return subscribe(conn, exchange, queueName, key, queueType, handler, gobUnmarshaller)
}

func subscribe[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	simpleQueueType SimpleQueueType,
	handler func(T) AckType,
	unmarshaller func([]byte) (T, error),
) error {

	channel, _, err := DeclareAndBind(conn, exchange, queueName, key, simpleQueueType)
	if err != nil {
		return err
	}
	
	err = channel.Qos(10, 0, true)
	if err != nil {
		return err
	}

	delivery, err := channel.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for d := range(delivery) {
			msg, err := unmarshaller(d.Body);
			if err != nil {
				fmt.Printf("could not unmarshal message: %v\n", err)
				continue
			}
			ackType := handler(msg)

			switch ackType {
			case Ack:
				d.Ack(false)
				fmt.Println("Ack")
			case NackRequeue:
				d.Nack(false, true)
				fmt.Println("NackReque")
			case NackDiscard:
				d.Nack(false, false)
				fmt.Println("NackDiscard")
			}
		}
	}()

	return nil
}

func jsonUnmarshaller[T any](data []byte) (T, error) {
	var v T
	err := json.Unmarshal(data, &v)
	return v, err
}

func gobUnmarshaller[T any](data []byte) (T, error) {
	var v T

	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&v)

	return v, err
}

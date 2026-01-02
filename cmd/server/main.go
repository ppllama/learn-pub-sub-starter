package main

import (
	"fmt"
	"log"

	// "os"
	// "os/signal"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)


func main() {
	fmt.Println("Starting Peril server...")
	const connString = "amqp://guest:guest@localhost:5672/"
	conn, err := amqp.Dial(connString)
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}
	defer conn.Close()
	fmt.Println("Connection was Successful")

	newChan, err := conn.Channel()
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}

	// _, _, err = pubsub.DeclareAndBind(conn, routing.ExchangePerilTopic, routing.GameLogSlug, "game_logs.*", pubsub.QueueDurable)
	// if err != nil {
	// 	log.Fatalf("it erred: %v", err)
	// }

	err = pubsub.SubscribeGob(conn, routing.ExchangePerilTopic, routing.GameLogSlug, "game_logs.*", pubsub.QueueDurable, handlerLog())
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}

	gamelogic.PrintServerHelp()
	for {
		input := gamelogic.GetInput()
		if len(input) == 0 {
			continue
	    }

		if input[0] == "pause" {
			fmt.Println("Sending a pause message")
			pubsub.PublishJSON(newChan, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: true})
		} else if input[0] == "resume" {
			fmt.Println("Sending a resume message")
			pubsub.PublishJSON(newChan, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: false})
		} else if input[0] == "quit" {
			break
		} else {
			fmt.Println("Unknown Command")
		}
	}

	
	
	// sigChan := make(chan os.Signal, 1)
	// signal.Notify(sigChan, os.Interrupt)
	// fmt.Println("Waiting for signal (Ctrl+C to exit)...")

	// // BLOCK here until a signal is received
	// <-sigChan

	fmt.Println("Shutting down gracefully...")

}


func handlerLog() func(gl routing.GameLog) pubsub.AckType{
	return func(gl routing.GameLog) pubsub.AckType{
		defer fmt.Print("> ")
		err := gamelogic.WriteLog(gl)
		if err != nil {
			fmt.Printf("Error writing log: %v\n", err)
			return pubsub.NackRequeue
		}
		return pubsub.Ack
	}
	
}
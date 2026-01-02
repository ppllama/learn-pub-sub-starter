package main

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril client...")
	const connString = "amqp://guest:guest@localhost:5672/"
	conn, err := amqp.Dial(connString)
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}
	defer conn.Close()
	fmt.Println("Connection was Successful")

	username, err := gamelogic.ClientWelcome()
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}

	_, _, err = pubsub.DeclareAndBind(conn, routing.ExchangePerilDirect, fmt.Sprintf("%s.%s", routing.PauseKey, username), routing.PauseKey, pubsub.QueueTansient)
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}
	armyChannel, _, err := pubsub.DeclareAndBind(conn, routing.ExchangePerilTopic, fmt.Sprintf("%s.%s", routing.ArmyMovesPrefix, username), fmt.Sprintf("%s.*", routing.ArmyMovesPrefix), pubsub.QueueTansient)
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}

	gameState := gamelogic.NewGameState(username)

	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilDirect, fmt.Sprintf("%s.%s", routing.PauseKey, username), routing.PauseKey, pubsub.QueueTansient, handlerPause(gameState))
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}
	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, fmt.Sprintf("%s.%s", routing.ArmyMovesPrefix, username), fmt.Sprintf("%s.*", routing.ArmyMovesPrefix), pubsub.QueueTansient, handlerMove(gameState, newChan(conn)))
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}
	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix, fmt.Sprintf("%s.*", routing.WarRecognitionsPrefix), pubsub.QueueDurable, handlerWar(gameState, newChan(conn)))
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}

	for {
	input := gamelogic.GetInput()
	if input[0] == "spawn" {
		if err := gameState.CommandSpawn(input); err != nil {
			fmt.Println(err)
		}

	} else if input[0] == "move" {
		armyMove, err := gameState.CommandMove(input)
		if err != nil {
			fmt.Println(err)
		}
		err = pubsub.PublishJSON(armyChannel, routing.ExchangePerilTopic, fmt.Sprintf("%s.%s", routing.ArmyMovesPrefix, username), armyMove)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("move was published successfully")

	} else if input[0] == "status" {
		gameState.CommandStatus()

	} else if input[0] == "help" {
		gamelogic.PrintClientHelp()

	} else if input[0] == "spam" {
		if len(input) < 2 {
			fmt.Println("usage: spam <number>")
		}
		spawnNo, err := strconv.Atoi(input[1])

		if err != nil {
			fmt.Println("Unable to parse int")
		}
		spamchan := newChan(conn)
		for range(spawnNo) {
			msg := gamelogic.GetMaliciousLog()
			log := routing.GameLog{
				CurrentTime: time.Now(),
				Message: msg,
				Username: gameState.GetUsername(),
			}
			publishGameLog(spamchan, log)
		}
		fmt.Printf("Spammed %d times\n", spawnNo)

	} else if input[0] == "quit" {
		gamelogic.PrintQuit()
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

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType{
	return func(ps routing.PlayingState) pubsub.AckType{
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.Ack
	}
}



func handlerMove(gs *gamelogic.GameState, publishCh *amqp.Channel) func(gamelogic.ArmyMove) pubsub.AckType {
	return func(gl gamelogic.ArmyMove) pubsub.AckType{
		defer fmt.Print("> ")
		outcome := gs.HandleMove(gl)
		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			err := pubsub.PublishJSON(publishCh, routing.ExchangePerilTopic, fmt.Sprintf("%s.%s", routing.WarRecognitionsPrefix, gs.GetUsername()), gamelogic.RecognitionOfWar{Attacker: gl.Player, Defender: gs.GetPlayerSnap()})
			if err != nil {
				fmt.Printf("error publishing war recognition")
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		default:
			return pubsub.NackDiscard
		}
	}
}


func handlerWar(gs *gamelogic.GameState, publishCh *amqp.Channel) func(gamelogic.RecognitionOfWar) pubsub.AckType {
	return func(gl gamelogic.RecognitionOfWar) pubsub.AckType{
	defer fmt.Print("> ")
	outcome, winner, loser := gs.HandleWar(gl)
	
	switch outcome {
	case gamelogic.WarOutcomeNotInvolved:
		return pubsub.NackRequeue
	case gamelogic.WarOutcomeNoUnits:
		return pubsub.NackDiscard
	case gamelogic.WarOutcomeOpponentWon:
		gameLog := routing.GameLog{
			CurrentTime: time.Now(),
			Message: fmt.Sprintf("%s won a war against %s", winner, loser),
			Username: gs.GetUsername(),
		}
		return publishGameLog(publishCh, gameLog)
	case gamelogic.WarOutcomeYouWon:
		gameLog := routing.GameLog{
			CurrentTime: time.Now(),
			Message: fmt.Sprintf("%s won a war against %s", winner, loser),
			Username: gs.GetUsername(),
		}
		return publishGameLog(publishCh, gameLog)
	case gamelogic.WarOutcomeDraw:
		gameLog := routing.GameLog{
			CurrentTime: time.Now(),
			Message: fmt.Sprintf("A war between %s and %s resulted in a draw", winner, loser),
			Username: gs.GetUsername(),
		}
		return publishGameLog(publishCh, gameLog)
	default:
		fmt.Printf("Waroutcome unrecognised and discarded: %v\n", outcome)
		return pubsub.NackDiscard
	}
	}

}

func newChan(a *amqp.Connection) *amqp.Channel {
	newCh, err := a.Channel()
	if err != nil {
		log.Fatalf("it erred: %v", err)
	}
	return newCh
}

func publishGameLog(publishCh *amqp.Channel, val routing.GameLog) pubsub.AckType {
	err := pubsub.PublishGob(publishCh, routing.ExchangePerilTopic, fmt.Sprintf("%s.%s", routing.GameLogSlug, val.Username), val)
	if err != nil {
		fmt.Printf("error publishing game log: %v\n", err)
		return pubsub.NackRequeue
	}
	return pubsub.Ack
}
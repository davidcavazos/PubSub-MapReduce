package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"./util"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
)

var NAME = "flood"
var PUB_DELAY = 100 * time.Millisecond

func main() {
	// Command line arguments
	project := flag.String(
		"project", os.Getenv("PROJECT"), "Google Cloud Project ID")
	inputTopic := flag.String(
		"input-topic", os.Getenv("FLOOD_TOPIC"), "Input topic")
	outputTopic := flag.String(
		"output-topic", os.Getenv("MAPPER_TOPIC"), "Output topic")
	flag.Parse()

	// Create the clients
	ctx := context.Background()

	iotClient, err := pubsub.NewClient(ctx, *project)
	if err != nil {
		log.Fatalf("%v: pubsub.NewClient: %v", NAME, err)
	}

	// Get input subscription
	floodSub, err := util.GetOrCreateSubscription(
			ctx, iotClient, *inputTopic, *inputTopic)
	if err != nil {
		log.Fatalf("%v: GetOrCreateSubscription: %v", NAME, err)
	}

	// Get output topic
	mapperTopic, err := util.GetOrCreateTopic(ctx, iotClient, *outputTopic)
	if err != nil {
		log.Fatalf("%v: GetOrCreateTopic: %v", NAME, err)
	}
	mapperTopic.PublishSettings = pubsub.PublishSettings{
		DelayThreshold: PUB_DELAY,
		CountThreshold: pubsub.MaxPublishRequestCount,
		ByteThreshold:  pubsub.MaxPublishRequestBytes,
	}

	// Receive messages
	fmt.Printf("%v: listening...\n", NAME)
	err = floodSub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println(r)
			}
			m.Ack()
		}()

		// Multiply the messages
		var flood util.FloodMessage
		err = json.Unmarshal([]byte(m.Data), &flood)
		if err != nil {
			log.Printf("%v: json.Unmarshal: %v\n", NAME, err)
		}

		anySent := false
		for id, n := range flood.Ns {
			for i := int64(0); i < n; i++ {
				bytes := []byte(strconv.FormatInt(int64(id), 16))
				mapperTopic.Publish(ctx, &pubsub.Message{Data: bytes})
				anySent = true
			}
		}
		if anySent {
			log.Printf("%v: flood=%v\n", NAME, flood)
		}
	})
	if err != nil {
		log.Fatalf("%v: sub.Receive: %v", NAME, err)
	}
}

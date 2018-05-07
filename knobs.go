package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"./util"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
)

var NAME = "knobs"
var PUB_DELAY = 200 * time.Millisecond
var TIME_DELTA = PUB_DELAY.Seconds()  / time.Second.Seconds()

func main() {
	// Command line arguments
	project := flag.String(
		"project", os.Getenv("PROJECT"), "Google Cloud Project ID")
	knobsTopicName := flag.String(
		"knobs-topic", "knobs", "Knobs topic")
	floodTopicName := flag.String(
		"flood-topic", "flood", "Flood topic")
	flag.Parse()

	// Creates the clients
	ctx := context.Background()

	iotClient, err := pubsub.NewClient(ctx, *project)
	if err != nil {
		log.Fatalf("%v: pubsub.NewClient: %v", NAME, err)
	}

	// Get input subscription
	knobsSub, err := util.GetOrCreateSubscription(
			ctx, iotClient, *knobsTopicName, *knobsTopicName)
	if err != nil {
		log.Fatalf("%v: GetOrCreateSubscription: %v", NAME, err)
	}

	// Get output topic
	floodTopic, err := util.GetOrCreateTopic(ctx, iotClient, *floodTopicName)
	if err != nil {
		log.Fatalf("%v: GetOrCreateTopic: %v", NAME, err)
	}
	floodTopic.PublishSettings = pubsub.PublishSettings{
		DelayThreshold: PUB_DELAY,
		CountThreshold: pubsub.MaxPublishRequestCount,
		ByteThreshold:  pubsub.MaxPublishRequestBytes,
	}

	// Every second, send a flood message with the latest known state of the knobs
	latestKnobs := make([]util.KnobsMessage, util.TOTAL_KNOBS)
	go func() {
		for _ = range time.Tick(PUB_DELAY) {
			windowTotal := int64(0)
			flood := util.FloodMessage{make([]int64, util.TOTAL_KNOBS)}
			for id, knob := range latestKnobs {
				delta := int64(float64(knob.N) * TIME_DELTA)
				flood.Ns[id] = delta
				windowTotal += delta
			}
			if windowTotal == 0 {
				continue
			}

			bytes, err := json.Marshal(flood)
			if err != nil {
				log.Printf("%v: json.Marshal: %v\n", NAME, err)
				continue
			}
			log.Printf("%v: flood=%v; windowTotal=%v\n", NAME, flood, windowTotal)
			floodTopic.Publish(ctx, &pubsub.Message{Data: bytes})
		}
	}()

	// Receive messages
	fmt.Printf("%v: listening...\n", NAME)
	err = knobsSub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println(r)
			}
			m.Ack()
		}()

		// Update the latest known state of the knobs
		var knob map[string]interface{}
		err = json.Unmarshal([]byte(m.Data), &knob)
		if err != nil {
			log.Printf("%v: json.Unmarshal: %v\n", NAME, err)
		}
		id := int64(knob["id"].(float64))
		n := int64(knob["n"].(float64))
		timestamp := knob["ts"].(float64)
		if latestKnobs[id].Timestamp < timestamp {
			latestKnobs[id] = util.KnobsMessage{n, timestamp}
		}
	})
	if err != nil {
		log.Fatalf("%v: sub.Receive: %v", NAME, err)
	}
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"./util"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
)

var NAME = "map"
var NUM_GOROUTINES = runtime.NumCPU() * 2
var PUB_DELAY = 100 * time.Millisecond

func main() {
	// Command line arguments
	project := flag.String(
		"project", os.Getenv("PROJECT"), "Google Cloud Project ID")
	inputTopic := flag.String(
		"input-topic", os.Getenv("MAPPER_TOPIC"), "Input topic")
	outputTopic := flag.String(
		"output-topic", os.Getenv("REDUCER_TOPIC"), "Output topic")
	flag.Parse()

	// Create the clients
	ctx := context.Background()

	iotClient, err := pubsub.NewClient(ctx, *project)
	if err != nil {
		log.Fatalf("%v: pubsub.NewClient: %v", NAME, err)
	}

	// Get input subscription
	mapperSub, err := util.GetOrCreateSubscription(
			ctx, iotClient, *inputTopic, *inputTopic)
	if err != nil {
		log.Fatalf("%v: GetOrCreateSubscription: %v", NAME, err)
	}
	mapperSub.ReceiveSettings = pubsub.ReceiveSettings{
		MaxOutstandingMessages: 1000000,
		NumGoroutines:          NUM_GOROUTINES,
	}

	// Get output topic
	reducerTopic, err := util.GetOrCreateTopic(ctx, iotClient, *outputTopic)
	if err != nil {
		log.Fatalf("%v: GetOrCreateTopic: %v", NAME, err)
	}
	reducerTopic.PublishSettings = pubsub.PublishSettings{
		DelayThreshold: PUB_DELAY,
		CountThreshold: pubsub.MaxPublishRequestCount,
		ByteThreshold:  pubsub.MaxPublishRequestBytes,
	}

	// Count how many messages came in through the window duration and publish it
	counters := make([]int64, util.TOTAL_KNOBS)
	go func() {
		for _ = range time.Tick(PUB_DELAY) {
			var windowTotal int64 = 0
			messages := make([]int64, util.TOTAL_KNOBS)
			for id := 0; id < util.TOTAL_KNOBS; id++ {
				messages[id] = atomic.SwapInt64(&counters[id], 0)
				windowTotal += messages[id]
			}

			if windowTotal > 0 {
				reducer := util.ReducerMessage{messages}
				bytes, err := json.Marshal(reducer)
				if err != nil {
					log.Printf("%v: json.Marshal: %v\n", NAME, err)
					continue
				}
				reducerTopic.Publish(ctx, &pubsub.Message{Data: bytes})
				log.Printf("%v: messages=%v\n", NAME, messages)
			}
		}
	}()

	// Receive messages
	fmt.Printf("%v: listening (%v goroutines)...\n", NAME, NUM_GOROUTINES)
	err = mapperSub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println(r)
			}
			m.Ack()
		}()

		// Increment the number of messages received per knob
		id, err := strconv.ParseInt(string(m.Data), 16, 8)
		if err != nil {
			log.Printf("%v: strconv.ParseInt: %v\n", NAME, err)
		}
		atomic.AddInt64(&counters[id], 1)
	})
	if err != nil {
		log.Fatalf("%v: sub.Receive: %v", NAME, err)
	}
}

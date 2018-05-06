package main

import (
	"encoding/json"
	"flag"
	"log"
	"math"
	"os"
	"os/signal"
	"sync"
	"time"

	"./util"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
)

var NAME = "simulate-knobs"
var MESSAGE_INTERVAL = 250 * time.Millisecond
var CYCLE_DURATION_SEC int64 = 20

func publish(ctx context.Context, topic *pubsub.Topic, id int, n int64) {
	// Wrapper function to publish a flood message
	knob := map[string]interface{}{
		"id": id,
		"n": n,
		"ts": float64(time.Now().UnixNano()) / 1000000000,
	}
	bytes, err := json.Marshal(knob)
	if err != nil {
		log.Printf("%v: json.Marshal: %v\n", NAME, err)
	}
	log.Printf("%v: knob=%v\n", NAME, string(bytes))
	topic.Publish(ctx, &pubsub.Message{Data: bytes})
}

func main() {
	// Command line arguments
	project := flag.String(
		"project", os.Getenv("PROJECT"), "Google Cloud Project ID")
	knobsTopicName := flag.String(
		"knobs-topic", "knobs", "Output topic")
	n := flag.Int64("n", 10000, "Number of messages")
	cycle := flag.Bool("cycle", false, "Make the messages cycle in a sine wave")
	flag.Parse()

	// Create the clients
	ctx := context.Background()

	iotClient, err := pubsub.NewClient(ctx, *project)
	if err != nil {
		log.Fatalf("%v: pubsub.NewClient: %v", NAME, err)
	}

	// Get output topic
	knobsTopic, err := util.GetOrCreateTopic(ctx, iotClient, *knobsTopicName)
	if err != nil {
		log.Fatalf("%v: GetOrCreateTopic: %v", NAME, err)
	}

	// Make the producer function for the knobs
	producerFn := func(id int64) int64 {return *n}
	if *cycle {
		producerFn = func(id int64) int64 {
			now := float64(time.Now().UnixNano()) / 1000000000.0
			interval := float64(CYCLE_DURATION_SEC * (id + 1))
			x := math.Mod(now, interval) / interval * math.Pi
			return int64(float64(*n) * math.Sin(x))
		}
	}

	// Simulate the knobs
	stop := false
	var wg sync.WaitGroup
	for id := 0; id < util.TOTAL_KNOBS; id++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for _ = range time.Tick(time.Duration(id + 1) * MESSAGE_INTERVAL) {
				if stop {
					return
				}
				publish(ctx, knobsTopic, id, producerFn(int64(id)))
			}
		}(id)
	}

	// Prepare to turn off all knobs when we exit with Ctrl-C
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	select {
	case sig := <-c:
		stop = true
		log.Printf("%v: signal %v caught, turning off knobs", NAME, sig)
		wg.Wait()
		for id := 0; id < util.TOTAL_KNOBS; id++ {
			publish(ctx, knobsTopic, id, 0)
		}
		// Sleep to make sure all the messages are published before exiting
		time.Sleep(time.Second)
	}
}

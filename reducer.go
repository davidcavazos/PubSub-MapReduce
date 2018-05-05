package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"./util"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	cloudiot "google.golang.org/api/cloudiot/v1"
)

var NAME = "reduce"
var NUM_GOROUTINES = runtime.NumCPU() * 2
var IOT_RETRIES = 10
var IOT_RETRY_WAIT = 100 * time.Millisecond

func main() {
	// Command line arguments
	project := flag.String(
		"project", os.Getenv("PROJECT"), "Google Cloud Project ID")
	reducerTopicName := flag.String(
		"reducer-topic", "reducer", "Reducer topic")
	region := flag.String(
		"region", "us-central1", "IoT Core Region")
	registry := flag.String(
		"registry", "registry", "IoT Core Registry")
	deviceID := flag.String(
		"device", "device", "IoT Core Device ID")
	skipIoT := flag.Bool("skip-iot", false, "Skip updating to IoT Core")
	flag.Parse()

	// Create the clients
	ctx := context.Background()

	pubsubClient, err := pubsub.NewClient(ctx, *project)
	if err != nil {
		log.Fatalf("%v: pubsub.NewClient: %v", NAME, err)
	}

	httpClient, err := google.DefaultClient(ctx, cloudiot.CloudPlatformScope)
	if err != nil {
		log.Fatalf("%v: google.DefaultClient: %v", NAME, err)
	}

	iotClient, err := cloudiot.New(httpClient)
	if err != nil {
		log.Fatalf("%v: cloudiot.New: %v", NAME, err)
	}

	// Get input subscription
	reducerSub, err := util.GetOrCreateSubscription(
			ctx, pubsubClient, *reducerTopicName, *reducerTopicName)
	if err != nil {
		log.Fatalf("%v: GetOrCreateSubscription: %v", NAME, err)
	}
	reducerSub.ReceiveSettings = pubsub.ReceiveSettings{
		MaxOutstandingMessages: 10000000,
		NumGoroutines:          NUM_GOROUTINES,
	}

	// Get output device
	iotPath := fmt.Sprintf("projects/%s/locations/%s/registries/%s/devices/%s",
		*project, *region, *registry, *deviceID)
	iotDevices := iotClient.Projects.Locations.Registries.Devices

	// Every second, update the messages per second and the total messages
	var counters = make([]int64, util.TOTAL_KNOBS)
	var mps = make([]int64, util.TOTAL_KNOBS)
	var total int64 = 0
	go func() {
		for _ = range time.Tick(time.Second) {
			var windowTotal int64 = 0
			for id := 0; id < util.TOTAL_KNOBS; id++ {
				mps[id] = atomic.SwapInt64(&counters[id], 0)
				windowTotal += mps[id]
			}
			atomic.AddInt64(&total, windowTotal)
		}
	}()

	// Every second push the latest metrics to IoT Core
	lastTotal := int64(0)
	go func() {
		for true {
			time.Sleep(time.Second)
			if total == lastTotal {
				continue
			}
			lastTotal = total

			data := map[string]interface{}{
				"mps": mps,
				"total": total,
			}
			bytes, err := json.Marshal(data)
			if err != nil {
				log.Printf("%v: json.Marshal: %v\n", NAME, err)
				continue
			}
			message := base64.StdEncoding.EncodeToString(bytes)
			request := cloudiot.ModifyCloudToDeviceConfigRequest{BinaryData: message}

			log.Printf("%v: mps=%v; total=%v", NAME, mps, total)
			if *skipIoT {
				continue
			}

			var response *cloudiot.DeviceConfig = nil
			for tries := 0; tries < IOT_RETRIES; tries++ {
				response, err = iotDevices.ModifyCloudToDeviceConfig(
					iotPath, &request).Do()
				if err == nil {
					break
				}
				time.Sleep(IOT_RETRY_WAIT)
			}
			if err != nil {
				log.Printf("%v: devices.ModifyCloudToDeviceConfig: %v\n", NAME, err)
				continue
			}
			fmt.Printf("  response=%v\n", response.Version)
		}
	}()

	// Receive messages
	fmt.Printf("%v: listening (%v goroutines)...\n", NAME, NUM_GOROUTINES)
	err = reducerSub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println(r)
			}
			m.Ack()
		}()

		// Add the reduced number of messages to get the total running count
		var data util.ReducerMessage
		err = json.Unmarshal([]byte(m.Data), &data)
		if err != nil {
			fmt.Printf("%v: json.Unmarshal:%v\n", NAME, err)
		}
		for id := 0; id < util.TOTAL_KNOBS; id++ {
			atomic.AddInt64(&counters[id], data.Messages[id])
		}
	})
	if err != nil {
		log.Fatalf("%v: sub.Receive: %v", NAME, err)
	}
}

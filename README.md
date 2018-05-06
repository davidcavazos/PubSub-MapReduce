# Pub/Sub MapReduce
The following sample shows how to create a Map-Reduce cluster using Pub/Sub for communication instead of the filesystem. With an appropriate cluster size, this can handle up to 1 million messages per second.

The basic workflow is the following. We have 5 knobs, each can point to anywhere between 0 and 200,000. The 5 knobs combined will handle up to 1,000,000. Those knobs are handled by a device in IoT Core, which will send every change done to any of the knobs through a Pub/Sub topic. We want to be flood that number of messages to another Pub/Sub topic, then count them, and send the results back the IoT Core device.

Here's a more detailed description of the workflow:
1. **IoT Core knobs**: publish to `knobs` topic
   Each knob publishes a message with their current value every time they change. The message includes the knob's ID (`id`), the number of messages to publish (`n`), and a timestamp of when that message was generated (`ts`).
   > `knobs`: `{"id": 1, "n": 42, "ts": 1525474779.668172}`

2. **`knobs.go`**: subscribes to `knobs` topic, publishes to `flood` topic
   This process is constantly listening for the updates of the knobs. Every second it's publishing a message with the latest state of each knob. This consists of a list of numbers, where each number is the number of messages that will be multiplied per knob.
   > `flood`: `{"Ns": [0, 42, 0, 0, 0]}`
   
3. **`flood.go`**: subscribes to `flood` topic, publishes to `mapper` topic
   Every time this process receives a message, it publishes the number of messages given in the list it received. Each message only contains the knob ID that originally sent it. This process is independent, so it can be run in parallel in a cluster for scale.
   > `mapper`: `1`

4. **`mapper.go`**: subscribes to `mapper` topic, publishes to `reducer` topic
   This process is constantly listening for the `mapper`'s messages. It counts them and classifies them by knob ID. Every 100 ms it sends a message with its local results. This process is independent, so it can be run in parallel in a cluster for scale.
   > `reducer`: `{"messages": [0, 21, 0, 0, 0]}`

5. **`reducer.go`**: subscribes to `reducer` topic, publishes to IoT Core
   This listens to the partial results of every reducer and aggregates all the results. It basically adds all the results from `reducer`, and every second it will publish the results of how many messages were processed in that second. The IoT Core publishing can be skipped with the `-skip-iot` flag.

## Requirements
Make sure you have the following installed:
- [Go language](https://golang.org/doc/install#install)
- [gcloud](https://cloud.google.com/sdk/downloads)
- [Docker](https://docs.docker.com/install/)

Enable the following Google Cloud products:
- [Kubernetes Engine](https://pantheon.corp.google.com/kubernetes)
- [Pub/Sub](https://pantheon.corp.google.com/cloudpubsub)
- [IoT Core](https://pantheon.corp.google.com/iot) *(optional)*

First, clone the repository and change directory to it.
```bash
git clone https://github.com/davidcavazos/PubSub-MapReduce.git
cd PubSub-MapReduce
```

## Local run
For this to work you'll have to run all the processes at the same time. The Pub/Sub topics and subscriptions will be created if they don't exist.

Make sure to your Compute Engine Service Account through the `GOOGLE_APPLICATION_CREDENTIALS` environment variable:
```bash
export GOOGLE_APPLICATION_CREDENTIALS=path/to/your/credentials.json
```

We now need to get the Go dependencies.
```bash
go get -u cloud.google.com/go/pubsub google.golang.org/api/cloudiot/v1
```

The following processes will have to be run on a different terminal or in a terminal multiplexer such as `tmux` or `screen`. Make sure the environment variables are set.
```bash
# You can either pass your Google CLoud Project ID with the -project flag
# on each process, or through the PROJECT environment variable
PROJECT=<Your Project ID>

# Process 1: subscribes to `knobs`, publishes to `flood`
go run knobs.go

# Process 2: subscribes to `flood`, publishes to `mapper`
go run flood.go

# Process 3: subscribes to `mapper`, publishes to `reducer`
go run mapper.go

# Process 4: subscribes to `reducer`, publishes to IoT Core
go run reducer.go -skip-iot
```

To send the knobs messages, you can use the included simulator to test things out.
```bash
# To make the knobs send a constant number
go run simulate-knobs.go -n 10000

# To make the knobs cycle through a sine wave, it goes from 0 to `n`
go run simulate-knobs.go -n 10000 -cycle
```

## Running on a Kubernetes cluster
First, we'll have to create a Kubernetes cluster, any size will do as long as there are enough resources for all the processes. To consistently get 1 million messages per second, we'll create an 18 nodes cluster, each VM will have 8 vCPUs with the minimum memory needed.
```bash
CLUSTER_NAME=cluster
PROJECT=<Your Project ID>

# Create the cluster
gcloud container clusters create $CLUSTER_NAME \
  --machine-type n1-highcpu-8 \
  --num-nodes 18

# Deploy the processes to Kubernetes
./deploy
```

To send the knobs messages, you can use the included simulator to test things out.
```bash
# To make the knobs send a constant number
go run simulate-knobs.go -n 200000

# To make the knobs cycle through a sine wave, it goes from 0 to `n`
go run simulate-knobs.go -n 200000 -cycle
```

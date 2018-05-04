package util

import (
	"fmt"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
)

var TOTAL_KNOBS = 5

type FloodMessage struct {
	Ns []int64
}

type KnobsMessage struct {
	N  int64
	Timestamp float64
}

type ReducerMessage struct {
	Messages []int64
}

func GetOrCreateTopic(
		ctx context.Context,
		client *pubsub.Client,
		topic_name string) (*pubsub.Topic, error) {

	topic := client.Topic(topic_name)
	if exists, err := topic.Exists(ctx); !exists {
		if err != nil {
			return nil, err
		}

		topic, err = client.CreateTopic(ctx, topic_name)
		if err != nil {
			return nil, err
		}
		fmt.Printf("created topic: %v\n", topic)
	}
	return topic, nil
}

func GetOrCreateSubscription(
		ctx context.Context,
		client *pubsub.Client,
		topic_name string,
		subscription_name string) (*pubsub.Subscription, error) {

	sub := client.Subscription(subscription_name)
	if exists, err := sub.Exists(ctx); !exists {
		if err != nil {
			return nil, err
		}

		topic, err := GetOrCreateTopic(ctx, client, topic_name)
		if err != nil {
			return nil, err
		}

		sub, err = client.CreateSubscription(ctx, subscription_name,
				pubsub.SubscriptionConfig{Topic: topic})
		if err != nil {
			return nil, err
		}
		fmt.Printf("created subscription: %v\n", sub)
	}
	return sub, nil
}

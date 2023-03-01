package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/centrifugal/centrifuge-go"
	"github.com/dgrijalva/jwt-go"
	"github.com/segmentio/encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

const tokenKey = "bbe7d157-a253-4094-9759-06a8236543f9"

func connToken(user string, exp int64) string {
	claims := jwt.MapClaims{"sub": user}
	if exp > 0 {
		claims["exp"] = exp
	}
	t, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(tokenKey))
	if err != nil {
		log.Println(err)
	}
	return t
}

type ChatMessage struct {
	Process string `json:"process"`
	Status  string `json:"status"`
}

func main() {
	go func() {
		log.Println(http.ListenAndServe(":8080", nil))
	}()
	client := centrifuge.NewJsonClient(
		"ws://localhost:8088/connection/websocket",
		centrifuge.Config{
			Token: connToken("user-4", 0),
		})
	defer client.Close()

	client.OnConnecting(func(e centrifuge.ConnectingEvent) {
		log.Printf("Connecting - %d (%s)", e.Code, e.Reason)
	})
	client.OnConnected(func(e centrifuge.ConnectedEvent) {
		log.Printf("Connected with ID %s", e.ClientID)
	})
	client.OnDisconnected(func(e centrifuge.DisconnectedEvent) {
		log.Printf("Disconnected: %d (%s)", e.Code, e.Reason)
	})

	client.OnError(func(e centrifuge.ErrorEvent) {
		log.Printf("Error: %s", e.Error.Error())
	})

	client.OnMessage(func(e centrifuge.MessageEvent) {
		log.Printf("Message from server: %s", string(e.Data))
	})

	client.OnSubscribed(func(e centrifuge.ServerSubscribedEvent) {
		log.Printf("Subscribed to server-side channel %s: (was recovering: %v, recovered: %v)", e.Channel, e.WasRecovering, e.Recovered)
	})
	client.OnSubscribing(func(e centrifuge.ServerSubscribingEvent) {
		log.Printf("Subscribing to server-side channel %s", e.Channel)
	})
	client.OnUnsubscribed(func(e centrifuge.ServerUnsubscribedEvent) {
		log.Printf("Unsubscribed from server-side channel %s", e.Channel)
	})

	client.OnPublication(func(e centrifuge.ServerPublicationEvent) {
		log.Printf("Publication from server-side channel %s: %s (offset %d)", e.Channel, e.Data, e.Offset)
	})
	client.OnJoin(func(e centrifuge.ServerJoinEvent) {
		log.Printf("Join to server-side channel %s: %s (%s)", e.Channel, e.User, e.Client)
	})
	client.OnLeave(func(e centrifuge.ServerLeaveEvent) {
		log.Printf("Leave from server-side channel %s: %s (%s)", e.Channel, e.User, e.Client)
	})

	err := client.Connect()
	if err != nil {
		log.Fatalln(err)
	}
	history, err := client.History(context.Background(), "parser-3", centrifuge.HistoryOption(func(options *centrifuge.HistoryOptions) {
		options.Limit = 100
		//options.Reverse = true

	}))
	for _, v := range history.Publications {
		fmt.Println(string(v.Data))
	}
	sub, err := client.NewSubscription("parser-3", centrifuge.SubscriptionConfig{
		Recoverable: true,
		JoinLeave:   true,
	})
	if err != nil {
		log.Fatalln(err)
	}

	sub.OnSubscribing(func(e centrifuge.SubscribingEvent) {
		log.Printf("Subscribing on channel %s - %d (%s)", sub.Channel, e.Code, e.Reason)
	})
	sub.OnSubscribed(func(e centrifuge.SubscribedEvent) {
		log.Printf("Subscribed on channel %s, (was recovering: %v, recovered: %v)", sub.Channel, e.WasRecovering, e.Recovered)
	})
	sub.OnUnsubscribed(func(e centrifuge.UnsubscribedEvent) {
		log.Printf("Unsubscribed from channel %s - %d (%s)", sub.Channel, e.Code, e.Reason)
	})

	sub.OnError(func(e centrifuge.SubscriptionErrorEvent) {
		log.Printf("Subscription error %s: %s", sub.Channel, e.Error)
	})
	sub.OnPublication(func(e centrifuge.PublicationEvent) {
		var chatMessage *ChatMessage
		err := json.Unmarshal(e.Data, &chatMessage)
		if err != nil {
			return
		}
		log.Printf("Someone says via channel %s: process: %s, status: %s, (offset %d)", sub.Channel, chatMessage.Process, chatMessage.Status, e.Offset)
	})
	sub.OnJoin(func(e centrifuge.JoinEvent) {
		log.Printf("Someone joined %s: user id %s, client id %s", sub.Channel, e.User, e.Client)
	})
	sub.OnLeave(func(e centrifuge.LeaveEvent) {
		log.Printf("Someone left %s: user id %s, client id %s", sub.Channel, e.User, e.Client)
	})

	err = sub.Subscribe()
	if err != nil {
		log.Fatalln(err)
	}

	_ = func(process, status string) error {
		msg := &ChatMessage{
			Process: process,
			Status:  status,
		}
		data, _ := json.Marshal(msg)
		_, err := sub.Publish(context.Background(), data)
		return err
	}

	log.Printf("Print something and press ENTER to send\n")

	// Read input from stdin.
	go func(sub *centrifuge.Subscription) {
		reader := bufio.NewReader(os.Stdin)
		for {
			text, _ := reader.ReadString('\n')
			text = strings.TrimSpace(text)
			switch text {
			case "#subscribe":
				err := sub.Subscribe()
				if err != nil {
					log.Println(err)
				}
			case "#unsubscribe":
				err := sub.Unsubscribe()
				if err != nil {
					log.Println(err)
				}
			case "#disconnect":
				err := client.Disconnect()
				if err != nil {
					log.Println(err)
				}
			case "#connect":
				err := client.Connect()
				if err != nil {
					log.Println(err)
				}
			case "#close":
				client.Close()
			default:
				//err = pubText(process, status)
				if err != nil {
					log.Printf("Error publish: %s", err)
				}
			}
		}
	}(sub)

	// Run until CTRL+C.
	select {}
}

// Iterate by 10.
//limit := 10
//// Paginate in reversed order first, then invert it.
//reverse := true
// Start with nil StreamPosition, then fill it with value while paginating.
//var sp *gocent.StreamPosition
//
//for {
//	historyResult, err = c.History(
//		ctx,
//		channel,
//		gocent.WithLimit(limit),
//		gocent.WithReverse(reverse),
//		gocent.WithSince(sp),
//	)
//	if err != nil {
//		log.Fatalf("Error calling history: %v", err)
//	}
//	for _, pub := range historyResult.Publications {
//		log.Println(pub.Offset, "=>", string(pub.Data))
//		sp = &gocent.StreamPosition{
//			Offset: pub.Offset,
//			Epoch:  historyResult.Epoch,
//		}
//	}
//	if len(historyResult.Publications) < limit {
//		// Got all pubs, invert pagination direction.
//		reverse = !reverse
//		log.Println("end of stream reached, change iteration direction")
//	}
//}

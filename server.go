// Copyright 2016 LINE Corporation
//
// LINE Corporation licenses this file to you under the Apache License,
// version 2.0 (the "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at:
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/line/line-bot-sdk-go/linebot"
)

const subConfrmType = "SubscriptionConfirmation"
const notificationType = "Notification"

func main() {
	app, err := NewKitchenSink(
		os.Getenv("CHANNEL_SECRET"),
		os.Getenv("CHANNEL_TOKEN"),
		os.Getenv("GROUP_ID"),
	)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/callback", app.Callback)
	http.HandleFunc("/testpush", app.TestPush)
	http.HandleFunc("/snspush", app.SnsPush)

	// This is just a sample code.
	// For actually use, you must support HTTPS by using `ListenAndServeTLS`, reverse proxy or etc.
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatal(err)
	}
}

// KitchenSink app
type KitchenSink struct {
	bot     *linebot.Client
	groupID string
}

// NewKitchenSink function
func NewKitchenSink(channelSecret, channelToken, groupID string) (*KitchenSink, error) {
	bot, err := linebot.New(
		channelSecret,
		channelToken,
	)
	if err != nil {
		return nil, err
	}
	return &KitchenSink{
		bot:     bot,
		groupID: groupID,
	}, nil
}

// Message Msg
type Message struct {
	Text string `json:"text"`
}

// SnsPush function for http server
func (app *KitchenSink) SnsPush(w http.ResponseWriter, r *http.Request) {
	var f interface{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Unable to Parse Body")
	}
	log.Printf(string(body))
	err = json.Unmarshal(body, &f)
	if err != nil {
		log.Printf("Unable to Unmarshal request")
	}

	data := f.(map[string]interface{})
	log.Println(data["Type"].(string))

	if data["Type"].(string) == subConfrmType {
		subcribeURL := data["SubscribeURL"].(string)
		go confirmSubscription(subcribeURL)
	} else if data["Type"].(string) == notificationType {
		log.Printf("Push message to %s: %s", app.groupID, data["Message"].(string))
		if _, err := app.bot.PushMessage(
			app.groupID,
			linebot.NewTextMessage(data["Message"].(string)),
		).Do(); err != nil {
			log.Print(err)
		}
	}
}

// TestPush function for http server
func (app *KitchenSink) TestPush(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var message Message
	if err := dec.Decode(&message); err != nil {
		log.Printf("error decoding message: %v", err)
		w.WriteHeader(400)
		return
	}

	log.Printf("TestPush message to %s: %s", app.groupID, message.Text)
	if _, err := app.bot.PushMessage(
		app.groupID,
		linebot.NewTextMessage(message.Text),
	).Do(); err != nil {
		log.Print(err)
	}
}

// Callback function for http server
func (app *KitchenSink) Callback(w http.ResponseWriter, r *http.Request) {
	events, err := app.bot.ParseRequest(r)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(400)
		} else {
			w.WriteHeader(500)
		}
		return
	}
	for _, event := range events {
		b, err := json.Marshal(event)
		if err != nil {
			log.Println("error:", err)
		}
		log.Printf("Got event %v", string(b))
		switch event.Type {
		case linebot.EventTypeMessage:
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				if err := app.handleText(message, event.ReplyToken, event.Source); err != nil {
					log.Print(err)
				}
			default:
				log.Printf("Unknown message: %v", message)
			}
		case linebot.EventTypeFollow:
			if err := app.replyText(event.ReplyToken, "Got followed event"); err != nil {
				log.Print(err)
			}
		case linebot.EventTypeUnfollow:
			log.Printf("Unfollowed this bot: %v", event)
		case linebot.EventTypeJoin:
			if err := app.replyText(event.ReplyToken, "Joined "+string(event.Source.Type)); err != nil {
				log.Print(err)
			}
		case linebot.EventTypeLeave:
			log.Printf("Left: %v", event)
		case linebot.EventTypePostback:
			data := event.Postback.Data
			if data == "DATE" || data == "TIME" || data == "DATETIME" {
				data += fmt.Sprintf("(%v)", *event.Postback.Params)
			}
			if err := app.replyText(event.ReplyToken, "Got postback: "+data); err != nil {
				log.Print(err)
			}
		case linebot.EventTypeBeacon:
			if err := app.replyText(event.ReplyToken, "Got beacon: "+event.Beacon.Hwid); err != nil {
				log.Print(err)
			}
		default:
			log.Printf("Unknown event: %v", event)
		}
	}
}

func (app *KitchenSink) handleText(message *linebot.TextMessage, replyToken string, source *linebot.EventSource) error {
	switch message.Text {
	case "bye":
		switch source.Type {
		case linebot.EventSourceTypeUser:
			return app.replyText(replyToken, "Bot can't leave from 1:1 chat")
		case linebot.EventSourceTypeGroup:
			if err := app.replyText(replyToken, "Leaving group"); err != nil {
				return err
			}
			if _, err := app.bot.LeaveGroup(source.GroupID).Do(); err != nil {
				return app.replyText(replyToken, err.Error())
			}
		case linebot.EventSourceTypeRoom:
			if err := app.replyText(replyToken, "Leaving room"); err != nil {
				return err
			}
			if _, err := app.bot.LeaveRoom(source.RoomID).Do(); err != nil {
				return app.replyText(replyToken, err.Error())
			}
		}
	}
	return nil
}

func (app *KitchenSink) replyText(replyToken, text string) error {
	if _, err := app.bot.ReplyMessage(
		replyToken,
		linebot.NewTextMessage(text),
	).Do(); err != nil {
		return err
	}
	return nil
}

func confirmSubscription(subcribeURL string) {
	response, err := http.Get(subcribeURL)
	if err != nil {
		fmt.Printf("Unbale to confirm subscriptions")
	} else {
		fmt.Printf("Subscription Confirmed sucessfully. %d", response.StatusCode)
	}
}

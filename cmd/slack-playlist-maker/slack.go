package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/zmb3/spotify"
)

func startSlackIntegration(ctx context.Context, tracks chan<- spotify.ID, hardErrorChan chan<- error, config Config) {
	api := slack.New(
		config.SlackToken,
		slack.OptionDebug(true),
		slack.OptionLog(log.New(os.Stdout, "[Slack] ", log.LstdFlags)),
		slack.OptionHTTPClient(&http.Client{
			Timeout: 10 * time.Second,
		}),
	)

	rtm := api.NewRTM(slack.RTMOptionUseStart(false))

	go rtm.ManageConnection()
	for {
		select {
		case <-ctx.Done():
			log.Println("Context done:", ctx.Err())
			return

		case msg, ok := <-rtm.IncomingEvents:
			if !ok {
				// all done
				log.Println("RTM IncomingEvents closed.")
				return
			}

			if err := processEvent(msg, tracks); err != nil {
				hardErrorChan <- err
			}
		}
	}
}

func processEvent(msg slack.RTMEvent, tracks chan<- spotify.ID) error {
	switch ev := msg.Data.(type) {
	case *slack.ConnectedEvent:
		log.Println("Infos:", ev.Info)
		log.Println("Connection counter:", ev.ConnectionCount)

	case *slack.MessageEvent:
		if err := processMessage(ev, tracks); err != nil {
			log.Println("Failed processing message:", ev)
			return nil
		}

	case *slack.RTMError:
		log.Printf("Error: %s\n", ev.Error())

	case *slack.InvalidAuthEvent:
		log.Println("Invalid credentials")
		return errors.New("invalid credentials")

	default:
		// ignore any other event
	}

	return nil
}

func processMessage(message *slack.MessageEvent, tracks chan<- spotify.ID) error {
	words := strings.Split(message.Text, " ")

	for _, word := range words {
		word = strings.Trim(word, "<>")
		trackID, ok := getSpotifyTrackLink(word)
		if !ok {
			continue
		}
		log.Println("Pushing track:", trackID)
		tracks <- trackID
	}

	return nil
}

func getSpotifyTrackLink(str string) (spotify.ID, bool) {
	// try parsing as a URL
	if u, err := url.Parse(str); err == nil {
		if !strings.Contains(u.Host, "spotify.com") && strings.Contains(u.Path, "/track/") {
			return "", false
		}

		parts := strings.Split(u.Path, "/")
		if len(parts) >= 2 {
			return spotify.ID(parts[len(parts)-1]), true
		}
	}

	return "", false
}

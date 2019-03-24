package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/urfave/cli"
)

type Config struct {
	SlackToken string `json:"-"`
}

func main() {
	var (
		config Config
		app    = cli.NewApp()
	)

	log.SetPrefix("[SPM] ")
	log.SetFlags(log.LstdFlags)
	log.SetOutput(os.Stdout)

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Usage:       "Specify the Slack Token",
			Name:        "slack-token",
			Value:       "",
			EnvVar:      "SLACK_TOKEN",
			Destination: &config.SlackToken,
		},
	}

	app.Action = func(_ *cli.Context) error {
		return run(config)
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalln("Exiting with error:", err)
	}
}

func run(config Config) error {
	api := slack.New(
		config.SlackToken,
		slack.OptionDebug(true),
		slack.OptionLog(log.New(os.Stdout, "[Slack] ", log.LstdFlags)),
		slack.OptionHTTPClient(&http.Client{
			Timeout: 10 * time.Second,
		}),
	)

	rtm := api.NewRTM(slack.RTMOptionUseStart(false))

	hardErrorChan := make(chan error)
	tracks := make(chan TrackID, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go rtm.ManageConnection()
	go processRTM(ctx, rtm, tracks, hardErrorChan)
	go processTracks(ctx, tracks)

	select {
	case err := <-hardErrorChan:
		log.Println("Got hard error, stopping:", err)
		cancel()
		return err
	}
}

func processRTM(ctx context.Context, rtm *slack.RTM, tracks chan TrackID, errorChan chan <-error) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Context done:", ctx.Err())
			return

		case msg, ok := <-rtm.IncomingEvents:
			if !ok {
				// all done
				log.Println("RTM IncomingEvents close.")
				return
			}

			if err := processEvent(msg, tracks); err != nil {
				errorChan <- err
			}
		}
	}
}

func processEvent(msg slack.RTMEvent, tracks chan TrackID) error {
	switch ev := msg.Data.(type) {
	case *slack.ConnectedEvent:
		fmt.Println("Infos:", ev.Info)
		fmt.Println("Connection counter:", ev.ConnectionCount)

	case *slack.MessageEvent:
		if err := processMessage(ev, tracks); err != nil {
			log.Println("Failed processing message:", ev)
			return nil
		}

	case *slack.RTMError:
		fmt.Printf("Error: %s\n", ev.Error())

	case *slack.InvalidAuthEvent:
		fmt.Println("Invalid credentials")
		return errors.New("invalid credentials")

	default:
		// ignore any other eent
	}

	return nil
}

func processMessage(message *slack.MessageEvent, tracks chan TrackID) error {
	words := strings.Split(message.Text, " ")

	for _, word := range words {
		word = strings.Trim(word, "<>")
		trackID, ok := getSpotifyTrackLink(word)
		if !ok {
			continue
		}
		tracks <- trackID
	}

	return nil
}

type TrackID string

func getSpotifyTrackLink(str string) (TrackID, bool) {
	// try parsing as a URL
	if u, err := url.Parse(str); err == nil {
		if !strings.Contains(u.Host, "spotify.com") && strings.Contains(u.Path, "/track/") {
			return "", false
		}

		parts := strings.Split(u.Path, "/")
		if len(parts) == 2 {
			return TrackID(parts[1]), true
		}
	}

	return "", false
}

func processTracks(ctx context.Context, tracks chan TrackID) {
	// todo
}
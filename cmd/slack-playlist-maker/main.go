package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
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

	go rtm.ManageConnection()

	for msg := range rtm.IncomingEvents {
		switch ev := msg.Data.(type) {
		case *slack.HelloEvent:
			// Ignore hello

		case *slack.ConnectedEvent:
			fmt.Println("Infos:", ev.Info)
			fmt.Println("Connection counter:", ev.ConnectionCount)
			// Replace C2147483705 with your Channel ID
			// rtm.SendMessage(rtm.NewOutgoingMessage("Hello world", "C2147483705"))

		case *slack.MessageEvent:
			if err := processMessage(ev); err != nil {
				log.Println("Failed processing message:", ev)
				continue
			}

		case *slack.PresenceChangeEvent:
			fmt.Printf("Presence Change: %v\n", ev)

		case *slack.LatencyReport:
			fmt.Printf("Current latency: %v\n", ev.Value)

		case *slack.RTMError:
			fmt.Printf("Error: %s\n", ev.Error())

		case *slack.InvalidAuthEvent:
			fmt.Println("Invalid credentials")
			return errors.New("invalid credentials")

		default:
			fmt.Printf("Other: %v\n", msg.Data)

			// Ignore other events..
			// fmt.Printf("Unexpected: %v\n", msg.Data)
		}

	}

	return nil
}

func processMessage(message *slack.MessageEvent) error {
	if len(message.Attachments) == 0 {
		log.Println("Message has no attachments, ignoring.")
		return nil
	}

	for _, attachment := range message.Attachments {
		spotifyAttachment, err := getSpotifyAttachment(attachment)
		if err != nil {
			log.Println("Error getting spotify attachment:", err)
			continue
		}

		log.Printf("Spotify Attachment found: %#v\n", spotifyAttachment)
	}

	return nil
}

type SpotifyAttachment struct {
	Service string `json:"service"`
	TitleLink string `json:"title_link"`
}

func getSpotifyAttachment(attachment slack.Attachment) (*SpotifyAttachment, error) {
	var sa SpotifyAttachment

	for _, field := range attachment.Fields {
		switch field.Title {
		case "service", "Service":
			if field.Value != "Spotify" {
				return nil, errors.New("not a spotify attachment")
			}
			sa.Service = field.Value
		case "title_link", "TitleLink":
			sa.TitleLink = field.Value
		}
	}

	if sa.Service == "" {
		return nil, errors.New("unknown attachment service")
	}

	return &sa, nil
}

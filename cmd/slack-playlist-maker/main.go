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
	"github.com/zmb3/spotify"
)

type Config struct {
	ListenAddress      string `json:"listen_address"`
	SlackToken         string `json:"-"`
	SpotifyID          string `json:"-"`
	SpotifySecret      string `json:"-"`
	SpotifyRedirectURI string `json:"spotify_redirect_uri"`
	SpotifyPlaylistID  string `json:"spotify_playlist_id"`
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
			Usage:       "Specify the listen address for the http server.",
			Name:        "listen-addr",
			Value:       ":8080",
			EnvVar:      "LISTEN_ADDR",
			Destination: &config.ListenAddress,
		},
		cli.StringFlag{
			Usage:       "Specify the Slack Token",
			Name:        "slack-token",
			Value:       "",
			EnvVar:      "SLACK_TOKEN",
			Destination: &config.SlackToken,
		},
		cli.StringFlag{
			Usage:       "Specify Spotify ID",
			Name:        "spotify-id",
			Value:       "",
			EnvVar:      "SPOTIFY_ID",
			Destination: &config.SpotifyID,
		},
		cli.StringFlag{
			Usage:       "Specify Spotify Secret",
			Name:        "spotify-secret",
			Value:       "",
			EnvVar:      "SPOTIFY_SECRET",
			Destination: &config.SpotifySecret,
		},
		cli.StringFlag{
			Usage:       "Specify Spotify Redirect URI",
			Name:        "spotify-redirect-uri",
			Value:       "http://localhost:8080/spotify/callback/",
			EnvVar:      "SPOTIFY_REDIRECT_URI",
			Destination: &config.SpotifyRedirectURI,
		},
		cli.StringFlag{
			Usage:       "Specify Spotify Playlist ID",
			Name:        "spotify-playlist-id",
			Value:       "",
			EnvVar:      "SPOTIFY_PLAYLIST_ID",
			Destination: &config.SpotifyPlaylistID,
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
	var (
		hardErrorChan = make(chan error)
		tracks        = make(chan spotify.ID, 10)
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go startSlackIntegration(ctx, tracks, hardErrorChan, config)
	time.Sleep(1 * time.Second)
	go startSpotifyIntegration(ctx, tracks, hardErrorChan, config)

	select {
	case err := <-hardErrorChan:
		log.Println("Got hard error, stopping:", err)
		cancel()
		return err
	}
}

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

package main

import (
	"context"
	"log"
	"os"
	"time"

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

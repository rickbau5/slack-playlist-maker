package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/zmb3/spotify"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func startSpotifyIntegration(ctx context.Context, tracks <-chan spotify.ID, hardErrors chan<- error, config Config) {
	client, err := awaitClient(ctx, config)
	if err != nil {
		hardErrors <- err
		return
	}
	for {
		select {
		case <-ctx.Done():
			log.Println("Context Done:", ctx.Err())
			return
		case track, ok := <-tracks:
			if !ok {
				log.Println("Spotify: Track chan closed, stopping")
				break
			}
			log.Println("Spotify: Got track:", track)
			if err := processTrack(client, track, spotify.ID(config.SpotifyPlaylistID)); err != nil {
				log.Println("Error processing track:", err)
				continue
			}
		}
	}
}

func awaitClient(ctx context.Context, config Config) (*spotify.Client, error) {
	var (
		auth           = spotify.NewAuthenticator(config.SpotifyRedirectURI, spotify.ScopePlaylistModifyPublic)
		clientChan     = make(chan *spotify.Client)
		spotifyHandler = NewSpotifyHandler(clientChan, auth)
		url            = auth.AuthURL(spotifyHandler.State())
		server         = NewServer(spotifyHandler, config.ListenAddress)
	)

	log.Println("Visit this link to login to Spotify:", url)

	var client *spotify.Client
	select {
	case client = <-clientChan:
	case <-ctx.Done():
		log.Println("Context closed before Spotify Client was acquired")
		return nil, ctx.Err()
	}
	if err := server.Stop(); err != nil {
		log.Println("Error stopping http server:", err)
	}

	if err := validateUser(client, spotify.ID(config.SpotifyPlaylistID)); err != nil {
		return nil, err
	}

	return client, nil
}

func validateUser(client *spotify.Client, playlistID spotify.ID) error {
	user, err := client.CurrentUser()
	if err != nil {
		log.Println("Failed getting user", err)
		return err
	}
	log.Printf("Logged in as '%s'.\n", user.ID)

	playlist, err := client.GetPlaylist(playlistID)
	if err != nil {
		log.Println("Failed getting playlist", err)
		return err
	}
	log.Printf("Playlist found: %s, is public: %v\n", playlist.Name, playlist.IsPublic)
	if !playlist.IsPublic {
		log.Println("Playlist is not public, cannot use it.")
		return fmt.Errorf("playlist not public: %s", playlist.Name)
	}

	return nil
}

func processTrack(client *spotify.Client, trackID spotify.ID, playlistID spotify.ID) error {
	track, err := client.GetTrack(trackID)
	if err != nil {
		return err
	}

	artists := make([]string, len(track.Artists))
	for i := range track.Artists {
		artists[i] = track.Artists[i].Name
	}

	log.Printf("Adding track: %v -> %s by %s\n", trackID, track.Name, strings.Join(artists, ", "))

	snapshotID, err := client.AddTracksToPlaylist(playlistID, trackID)
	if err != nil {
		return err
	}
	log.Printf("Successfully added track: %v. Snapshot: %v\n", trackID, snapshotID)

	return nil
}

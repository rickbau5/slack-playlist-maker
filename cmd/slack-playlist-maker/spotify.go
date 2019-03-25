package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/zmb3/spotify"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func startSpotifyIntegration(ctx context.Context, tracks <-chan spotify.ID, hardErrors chan<- error, config Config) {
	auth := spotify.NewAuthenticator(config.SpotifyRedirectURI, spotify.ScopePlaylistModifyPublic)
	clientChan := make(chan *spotify.Client)
	spotifyHandler := NewSpotifyHandler(clientChan, auth)
	mux := http.NewServeMux()
	mux.HandleFunc("/spotify/callback/", spotifyHandler.HandleAuthCallback)
	mux.HandleFunc("/healthcheck", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "\\0/")
	})
	server := http.Server{
		Addr:    config.ListenAddress,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			hardErrors <- err
			log.Println("HTTP Server encountered an error", err)
			return
		}
	}()

	url := auth.AuthURL(spotifyHandler.State())
	log.Println("Visit this link to login to Spotify:", url)

	client := <-clientChan
	user, err := client.CurrentUser()
	if err != nil {
		log.Println("Failed getting user", err)
		hardErrors <- err
		return
	}
	log.Printf("Logged in as '%s'.\n", user.ID)

	playlistID := spotify.ID(config.SpotifyPlaylistID)
	if err := validatePlaylist(client, playlistID); err != nil {
		hardErrors <- err
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Context Done:", ctx.Err())
			if err := server.Close(); err != nil {
				log.Println("Error closing server:", err)
			}
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

func validatePlaylist(client *spotify.Client, id spotify.ID) error {
	playlist, err := client.GetPlaylist(id)
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

func randomState(length int) string {
	runes := make([]rune, length)
	for i := range runes {
		runes[i] = letters[rand.Intn(len(letters))]
	}
	return string(runes)
}

type SpotifyHandler struct {
	auth       spotify.Authenticator
	state      string
	clientChan chan *spotify.Client
}

func NewSpotifyHandler(clientChan chan *spotify.Client, auth spotify.Authenticator) *SpotifyHandler {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	stateRunes := make([]rune, 16)
	for i := range stateRunes {
		stateRunes[i] = letters[random.Intn(len(letters))]
	}
	return &SpotifyHandler{
		auth:       auth,
		state:      string(stateRunes),
		clientChan: clientChan,
	}
}

func (h *SpotifyHandler) State() string {
	return h.state
}

func (h *SpotifyHandler) HandleAuthCallback(w http.ResponseWriter, req *http.Request) {
	token, err := h.auth.Token(h.state, req)
	if err != nil {
		http.Error(w, "Failed getting token", http.StatusBadRequest)
		log.Println("Failed getting token:", err)
		return
	}
	if st := req.FormValue("state"); st != h.state {
		http.NotFound(w, req)
		log.Printf("State does not match: %s != %s\n", h.state, st)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Login Successful"))

	client := h.auth.NewClient(token)
	h.clientChan <- &client
}

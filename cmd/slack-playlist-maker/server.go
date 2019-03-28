package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/zmb3/spotify"
)

type Server struct {
	server  *http.Server
	handler *SpotifyHandler

	startOnce, stopOnce sync.Once
}

func NewServer(handler *SpotifyHandler, listenAddr string) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/spotify/callback/", handler.HandleAuthCallback)
	mux.HandleFunc("/healthcheck", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "\\0/")
	})

	return &Server{
		server: &http.Server{
			Addr:    listenAddr,
			Handler: mux,
		},
		handler: handler,
	}
}

func (server *Server) Start() error {
	var err error
	server.startOnce.Do(func() {
		err = server.server.ListenAndServe()
	})

	return err
}

func (server *Server) Stop() error {
	var err error
	server.stopOnce.Do(func() {
		err = server.server.Close()
	})

	return err
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

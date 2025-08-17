// Package api implements the Hypcast HTTP API.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/gorilla/websocket"

	"github.com/featherbread/hypcast/internal/api/rpc"
	"github.com/featherbread/hypcast/internal/atsc/tuner"
)

var csrf = http.NewCrossOriginProtection()

var websocketUpgrader = &websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// [http.CrossOriginProtection] considers the GET requests used to start
		// WebSocket connections to be safe, even though the final connection might
		// not be "safe" in the HTTP sense.
		r = r.Clone(r.Context())
		r.Method = http.MethodPost
		return csrf.Check(r) == nil
	},
}

// Handler serves the Hypcast API for a single tuner.
type Handler struct {
	mux   *http.ServeMux
	tuner *tuner.Tuner
}

// NewHandler creates a Handler serving the Hypcast API for tuner.
func NewHandler(tuner *tuner.Tuner) *Handler {
	h := &Handler{
		mux:   http.NewServeMux(),
		tuner: tuner,
	}

	h.mux.HandleFunc("GET /api/config/channels", h.handleConfigChannels)

	// The RPC framework is expected to enforce its own method checks.
	rpcMux := http.NewServeMux()
	h.mux.Handle("/api/rpc/",
		csrf.Handler(
			rpc.WithLimitedBodyBuffer(1024,
				rpcMux)))
	rpcMux.Handle("/api/rpc/stop", rpc.Handle(h.rpcStop))
	rpcMux.Handle("/api/rpc/tune", rpc.Handle(h.rpcTune))

	// The websocket library is expected to enforce its own method checks.
	h.mux.HandleFunc("/api/socket/webrtc-peer", h.handleSocketWebRTCPeer)
	h.mux.HandleFunc("/api/socket/tuner-status", h.handleSocketTunerStatus)

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) handleConfigChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(slices.Collect(h.tuner.ChannelNames()))
}

func (h *Handler) rpcStop(r *http.Request, _ struct{}) (code int, body any) {
	slog.Info("Stopping tuner", "client", r.RemoteAddr)
	if err := h.tuner.Stop(); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusNoContent, nil
}

func (h *Handler) rpcTune(r *http.Request, params struct{ ChannelName string }) (code int, body any) {
	if params.ChannelName == "" {
		return http.StatusBadRequest, errors.New("channel name required")
	}

	slog.Info("Tuning to channel", "client", r.RemoteAddr, "channel", params.ChannelName)
	err := h.tuner.Tune(params.ChannelName)
	switch {
	case errors.Is(err, tuner.ErrChannelNotFound):
		return http.StatusBadRequest, err // Not 404; avoid confusion with nonexistent RPC route.
	case err != nil:
		return http.StatusInternalServerError, err
	}

	return http.StatusNoContent, nil
}

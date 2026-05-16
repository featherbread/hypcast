package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/featherbread/hypcast/internal/atsc/tuner"
	"github.com/featherbread/hypcast/internal/watch"
)

type TunerStatusHandler struct {
	log      *slog.Logger
	tuner    *tuner.Tuner
	ctx      context.Context
	shutdown context.CancelCauseFunc

	socket *websocket.Conn

	statusWatch watch.Watch
}

func (h *Handler) handleSocketTunerStatus(w http.ResponseWriter, r *http.Request) {
	ctx, shutdown := context.WithCancelCause(r.Context())
	tsh := &TunerStatusHandler{
		log:      slog.With("client", r.RemoteAddr),
		tuner:    h.tuner,
		ctx:      ctx,
		shutdown: shutdown,
	}
	tsh.ServeHTTP(w, r)
}

func (tsh *TunerStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tsh.log.Info("Connecting tuner status socket")
	defer func() {
		if tsh.statusWatch != nil {
			tsh.statusWatch.Wait()
		}
		tsh.log.Info("Disconnected tuner status socket", "error", context.Cause(tsh.ctx))
	}()

	if socket, err := websocket.Accept(w, r, nil); err == nil {
		tsh.socket = socket
	} else {
		return
	}

	defer tsh.socket.Close(websocket.StatusGoingAway, "server is shutting down")

	tsh.ctx = tsh.socket.CloseRead(tsh.ctx)

	tsh.statusWatch = tsh.tuner.WatchStatus(tsh.sendNewTunerStatus)
	defer tsh.statusWatch.Cancel()

	<-tsh.ctx.Done()
}

func (tsh *TunerStatusHandler) sendNewTunerStatus(s tuner.Status) {
	tsh.logTunerStatus(s)
	msg := tsh.mapTunerStatusToMessage(s)
	if err := wsjson.Write(tsh.ctx, tsh.socket, msg); err != nil {
		tsh.shutdown(err)
	}
}

func (tsh *TunerStatusHandler) logTunerStatus(s tuner.Status) {
	attrs := []slog.Attr{slog.Int("state", int(s.State))}
	if s.ChannelName != "" {
		attrs = append(attrs, slog.String("channel", s.ChannelName))
	}
	if s.Error != nil {
		attrs = append(attrs, slog.String("error", s.Error.Error()))
	}
	tsh.log.LogAttrs(tsh.ctx, slog.LevelInfo, "Sending tuner status", attrs...)
}

type tunerStatusMsg struct {
	State       string
	ChannelName string `json:",omitempty"`
	Error       string `json:",omitempty"`
}

var tunerStateStrings = map[tuner.State]string{
	tuner.StateStopped:  "Stopped",
	tuner.StateStarting: "Starting",
	tuner.StatePlaying:  "Playing",
}

func (tsh *TunerStatusHandler) mapTunerStatusToMessage(s tuner.Status) tunerStatusMsg {
	msg := tunerStatusMsg{
		State:       tunerStateStrings[s.State],
		ChannelName: s.ChannelName,
	}
	if s.Error != nil {
		msg.Error = s.Error.Error()
	}
	return msg
}

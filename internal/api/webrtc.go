package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"github.com/featherbread/hypcast/internal/atsc/tuner"
	"github.com/featherbread/hypcast/internal/watch"
)

var webrtcAPI *webrtc.API

func init() {
	// https://tools.ietf.org/html/rfc3551#section-3
	//
	// "This profile reserves payload type numbers in the range 96-127 exclusively
	// for dynamic assignment."
	const (
		videoPayloadType = 96 + iota
		audioPayloadType
	)

	var me webrtc.MediaEngine
	err := errors.Join(
		me.RegisterCodec(
			webrtc.RTPCodecParameters{PayloadType: videoPayloadType, RTPCodecCapability: tuner.VideoCodecCapability},
			webrtc.RTPCodecTypeVideo),
		me.RegisterCodec(
			webrtc.RTPCodecParameters{PayloadType: audioPayloadType, RTPCodecCapability: tuner.AudioCodecCapability},
			webrtc.RTPCodecTypeAudio),
	)
	if err != nil {
		panic(err)
	}

	webrtcAPI = webrtc.NewAPI(webrtc.WithMediaEngine(&me))
}

type WebRTCHandler struct {
	log       *slog.Logger
	tuner     *tuner.Tuner
	ctx       context.Context
	shutdown  context.CancelCauseFunc
	waitGroup sync.WaitGroup

	socket  *websocket.Conn
	rtcPeer *webrtc.PeerConnection
	watch   watch.Watch
}

func (h *Handler) handleSocketWebRTCPeer(w http.ResponseWriter, r *http.Request) {
	ctx, shutdown := context.WithCancelCause(r.Context())
	wh := &WebRTCHandler{
		log:      slog.With("client", r.RemoteAddr),
		tuner:    h.tuner,
		ctx:      ctx,
		shutdown: shutdown,
	}
	wh.ServeHTTP(w, r)
}

func (wh *WebRTCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wh.log.Info("Connecting WebRTC socket")
	defer func() {
		wh.waitForCleanup()
		wh.log.Info("Disconnected WebRTC socket", "error", context.Cause(wh.ctx))
	}()

	var err error
	wh.socket, err = websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer wh.socket.Close()

	wh.rtcPeer, err = webrtcAPI.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return
	}
	defer wh.rtcPeer.Close()

	wh.waitGroup.Go(wh.handleClientSessionAnswers)

	wh.watch = wh.tuner.WatchTracks(wh.handleTrackUpdate)
	defer wh.watch.Cancel()

	<-wh.ctx.Done()
}

func (wh *WebRTCHandler) handleClientSessionAnswers() {
	for {
		_, r, err := wh.socket.NextReader()
		if err != nil {
			wh.shutdown(err)
			return
		}

		var msg struct{ SDP webrtc.SessionDescription }
		if err := json.NewDecoder(r).Decode(&msg); err != nil {
			wh.shutdown(err)
			return
		}

		if err := wh.rtcPeer.SetRemoteDescription(msg.SDP); err != nil {
			wh.shutdown(err)
			return
		}
	}
}

func (wh *WebRTCHandler) handleTrackUpdate(ts tuner.Tracks) {
	wh.logTracks(ts)
	if err := wh.replaceTracks(ts); err != nil {
		wh.shutdown(err)
		return
	}
	if err := wh.renegotiateSession(); err != nil {
		wh.shutdown(err)
		return
	}
}

func (wh *WebRTCHandler) logTracks(ts tuner.Tracks) {
	var zero tuner.Tracks
	if ts == zero {
		wh.log.Info("Clearing WebRTC tracks")
	} else {
		wh.log.Info("Sending WebRTC tracks")
	}
}

func (wh *WebRTCHandler) replaceTracks(ts tuner.Tracks) error {
	if err := wh.removeTracks(); err != nil {
		return err
	}
	if ts == (tuner.Tracks{}) {
		return nil
	}
	return wh.addTracks(ts)
}

func (wh *WebRTCHandler) renegotiateSession() error {
	if !wh.hasTransceivers() {
		// Skip negotiation until we've had a chance to properly define video and
		// audio transceivers based on Tuner tracks.
		return nil
	}

	sdp, err := wh.rtcPeer.CreateOffer(nil)
	if err != nil {
		return err
	}

	// TODO: Should probably implement trickle ICE, but since Hypcast doesn't
	// implement STUN support it's not like ICE gathering takes much time.
	gatherComplete := webrtc.GatheringCompletePromise(wh.rtcPeer)

	if err := wh.rtcPeer.SetLocalDescription(sdp); err != nil {
		return err
	}

	<-gatherComplete
	msg := struct{ SDP webrtc.SessionDescription }{*wh.rtcPeer.LocalDescription()}
	return wh.socket.WriteJSON(msg)
}

func (wh *WebRTCHandler) removeTracks() error {
	for _, sender := range wh.rtcPeer.GetSenders() {
		if err := wh.rtcPeer.RemoveTrack(sender); err != nil {
			return err
		}
	}
	return nil
}

func (wh *WebRTCHandler) addTracks(ts tuner.Tracks) error {
	if wh.hasTransceivers() {
		return wh.addTracksWithExistingTransceivers(ts)
	}
	return wh.addTracksWithNewTransceivers(ts)
}

func (wh *WebRTCHandler) addTracksWithExistingTransceivers(ts tuner.Tracks) error {
	if _, err := wh.rtcPeer.AddTrack(ts.Video); err != nil {
		return err
	}
	if _, err := wh.rtcPeer.AddTrack(ts.Audio); err != nil {
		return err
	}
	return nil
}

func (wh *WebRTCHandler) addTracksWithNewTransceivers(ts tuner.Tracks) error {
	init := webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	}
	if _, err := wh.rtcPeer.AddTransceiverFromTrack(ts.Video, init); err != nil {
		return err
	}
	if _, err := wh.rtcPeer.AddTransceiverFromTrack(ts.Audio, init); err != nil {
		return err
	}
	return nil
}

func (wh *WebRTCHandler) hasTransceivers() bool {
	return len(wh.rtcPeer.GetTransceivers()) > 0
}

func (wh *WebRTCHandler) waitForCleanup() {
	if wh.watch != nil {
		wh.watch.Wait()
	}
	wh.waitGroup.Wait()
}

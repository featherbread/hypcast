// Package tuner implements an ATSC tuner that outputs WebRTC video and audio
// tracks.
package tuner

import (
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/featherbread/hypcast/internal/atsc"
	"github.com/featherbread/hypcast/internal/gst"
	"github.com/featherbread/hypcast/internal/watch"
)

// State represents the current state of the tuner.
type State int

const (
	// StateStopped means the tuner is switched off.
	StateStopped State = iota
	// StateStarting means that the tuner is trying to lock onto a signal and
	// start streaming.
	StateStarting
	// StatePlaying means that the tuner is locked onto a singal and is actively
	// streaming video.
	StatePlaying
)

// Status represents the public state of the tuner for reading by clients.
type Status struct {
	State       State
	ChannelName string
	Error       error
}

// Tracks represents the current set of video and audio tracks for use by WebRTC
// clients.
type Tracks struct {
	Video webrtc.TrackLocal
	Audio webrtc.TrackLocal
}

// VideoPipeline controls which pipeline Hypcast uses to process video.
type VideoPipeline string

const (
	// VideoPipelineDefault performs software-based video processing. It should
	// work on a wide variety of machines with little to no additional
	// configuration.
	VideoPipelineDefault VideoPipeline = "default"

	// VideoPipelineLowPower performs software-based video processing with limits
	// on bitrate, frame rate, and video size. This creates a smoother viewing
	// experience on servers with limited CPU power, but produces very
	// low-quality output.
	VideoPipelineLowPower VideoPipeline = "lowpower"

	// VideoPipelineVAAPI performs hardware accelerated video processing using
	// the Video Acceleration API (VA-API). It is more performant than the
	// default pipeline, but requires installation of gstreamer-vaapi plugins and
	// may require additional configuration to select an appropriate device and
	// driver. See GStreamer documentation for details.
	VideoPipelineVAAPI VideoPipeline = "vaapi"
)

// ParseVideoPipeline selects a VideoPipeline by name. Unknown names will return
// the default pipeline.
func ParseVideoPipeline(name string) VideoPipeline {
	switch {
	default:
		return VideoPipelineDefault
	case name == string(VideoPipelineLowPower):
		return VideoPipelineLowPower
	case name == string(VideoPipelineVAAPI):
		return VideoPipelineVAAPI
	}
}

// Tuner represents an ATSC tuner whose video and audio signals are encoded for
// use by WebRTC clients, and whose consumers are notified of ongoing state
// changes.
type Tuner struct {
	mu sync.Mutex

	channels   []atsc.Channel
	channelMap map[string]atsc.Channel

	videoPipeline VideoPipeline
	pipeline      *gst.Pipeline

	status *watch.Value[Status]
	tracks *watch.Value[Tracks]
}

// NewTuner creates a new Tuner that can tune to any of the provided channels.
func NewTuner(channels []atsc.Channel, videoPipeline VideoPipeline) *Tuner {
	return &Tuner{
		channels:      channels,
		channelMap:    makeChannelMap(channels),
		videoPipeline: videoPipeline,
		status:        watch.NewValue(Status{}),
		tracks:        watch.NewValue(Tracks{}),
	}
}

func makeChannelMap(channels []atsc.Channel) map[string]atsc.Channel {
	m := make(map[string]atsc.Channel, len(channels))
	for _, ch := range channels {
		m[ch.Name] = ch
	}
	return m
}

// ChannelNames returns an iterator over the names of channels that may be
// passed to [Tuner.Tune].
func (t *Tuner) ChannelNames() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, ch := range t.channels {
			if !yield(ch.Name) {
				break
			}
		}
	}
}

// WatchStatus sets up a handler function to continuously receive the status of
// the tuner as it is updated. See the watch package documentation for details.
func (t *Tuner) WatchStatus(handler func(Status)) watch.Watch {
	return t.status.Watch(handler)
}

// WatchTracks sets up a handler function to continuously receive the tuner's
// WebRTC tracks as they are updated. See the watch package documentation for
// details.
func (t *Tuner) WatchTracks(handler func(Tracks)) watch.Watch {
	return t.tracks.Watch(handler)
}

// Stop ends any active stream and releases the DVB device associated with this
// tuner.
func (t *Tuner) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	err := t.destroyAnyRunningPipeline()
	t.status.Set(Status{Error: err})
	t.tracks.Set(Tracks{})
	return err
}

// ErrChannelNotFound is returned when tuning to a channel whose name is not in
// the tuner's channel list.
var ErrChannelNotFound error = errors.New("channel not found")

// Tune attempts to start a stream for the named channel.
func (t *Tuner) Tune(channelName string) (err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	channel, ok := t.channelMap[channelName]
	if !ok {
		return ErrChannelNotFound
	}

	t.status.Set(Status{
		State:       StateStarting,
		ChannelName: channel.Name,
	})

	defer func() {
		if err != nil {
			t.destroyAnyRunningPipeline()
			t.status.Set(Status{Error: err})
		}
	}()

	t.destroyAnyRunningPipeline()

	t.pipeline, err = t.newPipeline(channel)
	if err != nil {
		return err
	}

	vt, at, err := t.createTrackPair()
	if err != nil {
		return err
	}

	t.pipeline.SetSink(sinkNameVideo, createTrackSink(vt))
	t.pipeline.SetSink(sinkNameAudio, createTrackSink(at))

	slog.Info("Starting transcode pipeline")
	err = t.pipeline.Start()
	if err != nil {
		return err
	}
	slog.Info("Started transcode pipeline")

	t.status.Set(Status{State: StatePlaying, ChannelName: channelName})
	t.tracks.Set(Tracks{Video: vt, Audio: at})
	return nil
}

func (t *Tuner) newPipeline(channel atsc.Channel) (*gst.Pipeline, error) {
	description, err := t.createPipelineDescription(channel)
	if err != nil {
		return nil, err
	}
	return gst.NewPipeline(description)
}

func (t *Tuner) createPipelineDescription(channel atsc.Channel) (string, error) {
	var buf strings.Builder

	err := pipelineDescriptionTemplate.Execute(&buf, struct {
		Modulation    string
		FrequencyHz   uint
		ProgramID     uint
		VideoPipeline string
	}{
		Modulation:    pipelineModulations[channel.Modulation],
		FrequencyHz:   channel.FrequencyHz,
		ProgramID:     channel.ProgramID,
		VideoPipeline: string(t.videoPipeline),
	})
	if err != nil {
		return "", fmt.Errorf("building pipeline template: %w", err)
	}

	return buf.String(), nil
}

var pipelineModulations = map[atsc.Modulation]string{
	atsc.Modulation8VSB:   "8vsb",
	atsc.ModulationQAM64:  "qam-64",
	atsc.ModulationQAM256: "qam-256",
}

const (
	sinkNameVideo = "video"
	sinkNameAudio = "audio"
)

var pipelineDescriptionTemplate = template.Must(template.New("").Parse(`
	dvbsrc delsys=atsc modulation={{.Modulation}} frequency={{.FrequencyHz}}
	{{- block "queue-max-time" 2_500_000_000 }}
	! queue leaky=downstream max-size-time={{.}} max-size-buffers=0 max-size-bytes=0
	{{- end }}
	! tsdemux name=demux program-number={{.ProgramID}} latency=500

	demux.
	{{- template "queue-max-time" 2_500_000_000 }}
	{{- if eq .VideoPipeline "vaapi" }}
	! vaapimpeg2dec
	! vaapipostproc deinterlace-mode=auto
	! vaapih264enc rate-control=cbr bitrate=12000 cpb-length=1000 quality-level=1 tune=high-compression
	{{- else }}
	{{- if eq .VideoPipeline "lowpower" }}
	! avdec_mpeg2video lowres=1/2-size
	{{- else }}
	! avdec_mpeg2video
	{{- end }}
	! deinterlace
	{{- if eq .VideoPipeline "lowpower" }}
	{{- template "queue-max-time" 2_500_000_000 }}
	! videorate max-rate=30
	! videoscale add-borders=true method=nearest-neighbour
	{{- template "queue-max-time" 2_500_000_000 }}
	! video/x-raw,width=640,height=360
	! x264enc bitrate=2500 vbv-buf-capacity=1000 speed-preset=ultrafast bframes=0 mb-tree=false key-int-max=60 rc-lookahead=30
	{{- else }}
	! x264enc bitrate=8000 vbv-buf-capacity=1000 speed-preset=ultrafast tune=zerolatency
	{{- end }}
	{{- end }}
	! video/x-h264,profile=constrained-baseline,stream-format=byte-stream
	! appsink name=video max-buffers=50 drop=true

	demux.
	{{- template "queue-max-time" 2_500_000_000 }}
	! a52dec
	! audioconvert
	! audioresample
	! audio/x-raw,rate=48000,channels=2
	! opusenc bitrate=128000
	! appsink name=audio max-buffers=50 drop=true
`))

func (t *Tuner) destroyAnyRunningPipeline() error {
	if t.pipeline == nil {
		return nil
	}
	err := t.pipeline.Close()
	t.pipeline = nil
	slog.Info("Destroyed transcode pipeline", "error", err)
	return err
}

// fmtp is described by https://tools.ietf.org/html/rfc6184.
//
// profile-level-id in particular is described in section 8.1 of the RFC. The
// first 2 octets together indicate the Constrained Baseline profile (42h to
// specify the Baseline profile, e0h to specify constraint set 1). The third
// octet (28h = 40) specifies level 4.0 (the level number times 10), the lowest
// to support 1920x1080 video per
// https://en.wikipedia.org/wiki/Advanced_Video_Coding#Levels.
//
// This needs to match up with the GStreamer pipeline definition.
const videoCodecFMTP = "profile-level-id=42e028;level-asymmetry-allowed=1;packetization-mode=1"

var (
	// VideoCodecCapability represents the RTP codec settings for the video signal
	// produced by the tuner.
	VideoCodecCapability = webrtc.RTPCodecCapability{
		MimeType:    webrtc.MimeTypeH264,
		ClockRate:   90_000,
		SDPFmtpLine: videoCodecFMTP,
	}

	// AudioCodecCapability represents the RTP codec settings for the audio signal
	// produced by the tuner.
	AudioCodecCapability = webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: 48_000,
		Channels:  2,
	}
)

func (t *Tuner) createTrackPair() (video, audio *webrtc.TrackLocalStaticSample, err error) {
	streamID := fmt.Sprintf("Tuner(%p)", t)
	video, verr := webrtc.NewTrackLocalStaticSample(VideoCodecCapability, streamID, streamID)
	audio, aerr := webrtc.NewTrackLocalStaticSample(AudioCodecCapability, streamID, streamID)
	err = errors.Join(verr, aerr)
	return
}

func createTrackSink(track *webrtc.TrackLocalStaticSample) gst.SinkFunc {
	return gst.SinkFunc(func(data []byte, duration time.Duration) {
		track.WriteSample(media.Sample{
			Data:     data,
			Duration: duration,
		})
	})
}

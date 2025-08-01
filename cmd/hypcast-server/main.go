package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/featherbread/hypcast/client"
	"github.com/featherbread/hypcast/internal/api"
	"github.com/featherbread/hypcast/internal/assets"
	"github.com/featherbread/hypcast/internal/atsc"
	"github.com/featherbread/hypcast/internal/atsc/tuner"
)

var (
	flagAddr          string
	flagChannels      string
	flagAssets        string
	flagVideoPipeline string
)

func init() {
	flag.StringVar(
		&flagAddr, "addr", ":9200",
		"Address for the HTTP server to listen on",
	)
	flag.StringVar(
		&flagChannels, "channels", "/etc/hypcast/channels.conf",
		"Path to the channels.conf file containing the list of available channels",
	)
	flag.StringVar(
		&flagAssets, "assets", "",
		"Path to client assets; overrides any embedded assets",
	)
	flag.StringVar(
		&flagVideoPipeline, "video-pipeline", "default",
		`Video pipeline implementation (default, lowpower, vaapi)`,
	)
}

func main() {
	flag.Parse()

	channels, err := readChannelsConf(flagChannels)
	if err != nil {
		slog.Error("Failed to load channels", "channels", flagChannels, "error", err)
		os.Exit(1)
	}

	vp := tuner.ParseVideoPipeline(flagVideoPipeline)
	tuner := tuner.NewTuner(channels, vp)
	http.Handle("/api/", api.NewHandler(tuner))

	var assetLogAttr slog.Attr
	if flagAssets != "" {
		assetLogAttr = slog.Group("assets", "path", flagAssets)
		http.Handle("/", http.FileServer(
			assets.FileSystem{FileSystem: http.Dir(flagAssets)},
		))
	} else if client.Build != nil {
		assetLogAttr = slog.Group("assets", "embedded", true)
		http.Handle("/", http.FileServer(
			assets.FileSystem{FileSystem: http.FS(client.Build)},
		))
	}

	slog.LogAttrs(
		context.Background(), slog.LevelInfo,
		"Starting Hypcast server",
		slog.String("addr", flagAddr),
		slog.String("channels", flagChannels),
		slog.String("pipeline", string(vp)),
		assetLogAttr,
	)
	server := http.Server{Addr: flagAddr}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		slog.Error("Failed to run Hypcast server", "error", err)
		os.Exit(1)

	case <-signalCh:
		slog.Info("Shutting down")
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(stopCtx)
	}
}

func readChannelsConf(path string) ([]atsc.Channel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return atsc.ParseChannelsConf(f)
}

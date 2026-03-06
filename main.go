package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/opensvc/om3/v3/core/client"
	"github.com/opensvc/om3/v3/core/event"
	"github.com/pkg/errors"
)

type (
	zoneRecordEvent struct {
		Path string
		Node string
		Name string
	}
)

var (
	lastGetEventReader time.Time
	lastDial           time.Time
	osvcSock           string
	pdnsSock           string
	connectInterval    time.Duration

	evHandlingTimeout = 300 * time.Millisecond
	watchdogTimeout   = 10 * time.Second
	logLevel          string

	defaultOSVCSock = "http:///var/run/lsnr/http.sock"
	defaultPDNSSock = "/var/run/pdns-recursor/pdns_recursor.controlsocket"
	defaultLogLevel = "info"
)

func main() {
	if s, ok := os.LookupEnv("OPENSVC_LSNR_SOCK"); ok {
		defaultOSVCSock = s
	}
	if s, ok := os.LookupEnv("OPENSVC_RECURSOR_SOCK"); ok {
		defaultPDNSSock = s
	}

	flag.StringVar(&osvcSock, "osvc-sock", defaultOSVCSock, "the unix domain socket of the opensvc agent api (http:///path/to.sock")
	flag.StringVar(&pdnsSock, "pdns-sock", defaultPDNSSock, "the unix domain socket of the power dns recursor api")
	flag.StringVar(&logLevel, "log", defaultLogLevel, "the log level (debug, info, warn, error)")
	flag.DurationVar(&connectInterval, "connect-interval", 2*time.Second, "the interval between socket reconnects")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	switch logLevel {
	case "debug":
		slog.SetLogLoggerLevel(slog.LevelDebug)
	case "info":
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case "warn":
		slog.SetLogLoggerLevel(slog.LevelWarn)
	case "error":
		slog.SetLogLoggerLevel(slog.LevelError)
	}

	for {
		if err := watch(); err != nil {
			slog.Error(fmt.Sprintf("watch: %s"))
		}
		time.Sleep(connectInterval)
	}
}

func reGetEventReader() (event.ReadCloser, error) {
	minGetEventReaderTime := lastGetEventReader.Add(connectInterval)
	if time.Now().Before(minGetEventReaderTime) {
		time.Sleep(minGetEventReaderTime.Sub(time.Now()))
	}
	lastGetEventReader = time.Now()
	return getEventReader()
}

func getEventReader() (event.ReadCloser, error) {
	cli, err := client.New(
		client.WithURL(osvcSock),
		client.WithTimeout(0),
	)
	if err != nil {
		return nil, errors.Wrap(err, "new client")
	}
	evReader, err := cli.NewGetEvents().SetFilters([]string{"ZoneRecordUpdated", "ZoneRecordDeleted", "WatchDog"}).NewTimeoutReader(watchdogTimeout)
	if err != nil {
		return nil, errors.Wrap(err, "new event reader")
	}
	return evReader, nil
}

func watch() error {
	var (
		ev       *event.Event
		evData   zoneRecordEvent
		evReader event.ReadCloser
	)

	q := make(chan zoneRecordEvent, 1)

	go func() {
		var (
			needWipeAll  bool
			err, lastErr error
		)

		for {
			if evReader != nil {
				ev, err = evReader.Read()
			}
			if evReader == nil || ev == nil || err != nil {
				if (err != nil) && ((lastErr == nil) || (lastErr.Error() != err.Error())) {
					lastErr = err
					slog.Error(fmt.Sprintf("event: read: %s", err))
				}
				if evReader != nil {
					_ = evReader.Close()
					if ev == nil {
						slog.Warn("event: read timeout")
					}
				}
				evReader, err = reGetEventReader()
				if err != nil {
					if (lastErr == nil) || (lastErr.Error() != err.Error()) {
						lastErr = err
						slog.Error(fmt.Sprintf("event: new reader: %s", err))
					}
				} else {
					slog.Info("event: new reader")
				}

				// now reconnected with the opensvc daemon, we don't know what we missed
				// during the unconnected period => wipe all to resync
				needWipeAll = true

				continue
			}
			if ev.Kind == "WatchDog" {
				continue
			}
			if err := json.Unmarshal(ev.Data, &evData); err != nil {
				slog.Error(fmt.Sprintf("event: unmarshal: %s on '%s'", err, ev.Data))
				continue
			}
			if needWipeAll {
				needWipeAll = false
				q <- zoneRecordEvent{Name: "."}
			} else {
				q <- evData
			}
		}
	}()
	for {
		ev := <-q
		if err := onEvent(ev); err != nil {
			slog.Error(fmt.Sprintf("on event: %s", err))
		}
	}
	if err := evReader.Close(); err != nil {
		slog.Error(fmt.Sprintf("close event reader: %s", err))
	}
	return nil
}

func onEvent(evData zoneRecordEvent) error {
	wiper := func() error {
		var lastErr error
		for {
			err := wipe(evData.Name)
			switch {
			case errors.Is(err, os.ErrDeadlineExceeded):
				slog.Error(fmt.Sprintf("pdns control socket: %s", err))
			case err != nil:
				if (lastErr == nil) || (err.Error() != lastErr.Error()) {
					slog.Error(fmt.Sprintf("wipe error: %s", err))
				}
				lastErr = err
				time.Sleep(300 * time.Millisecond)
			default:
				return nil
			}
		}
	}
	return wiper()
}

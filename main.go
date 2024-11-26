package main

import (
	"encoding/json"
	"flag"
	"os"
	"time"

	"github.com/opensvc/om3/core/client"
	"github.com/opensvc/om3/core/event"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

	log.Logger = log.Output(zerolog.NewConsoleWriter())
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	switch logLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}

	for {
		if err := watch(); err != nil {
			log.Error().Err(err).Msg("watch")
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
	evReader, err := cli.NewGetEvents().SetFilters([]string{"ZoneRecordUpdated", "ZoneRecordDeleted"}).GetReader()
	if err != nil {
		return nil, errors.Wrap(err, "new event reader")
	}
	return evReader, nil
}

func watch() error {
	var (
		ev     *event.Event
		evData zoneRecordEvent
	)

	evReader, err := getEventReader()
	if err != nil {
		return err
	}

	q := make(chan zoneRecordEvent, 1)

	go func() {
		for {
			if evReader != nil {
				ev, err = evReader.Read()
			}
			if evReader == nil || ev == nil || err != nil {
				log.Error().Err(err).Msg("reader")
				if evReader != nil {
					_ = evReader.Close()
				}
				evReader, err = reGetEventReader()
				if err != nil {
					log.Error().Err(err).Msg("reader: re-get event reader")
				}
				continue
			}
			if err := json.Unmarshal(ev.Data, &evData); err != nil {
				log.Error().Err(err).Msgf("reader: unmarshal error %s on '%s'", err, ev.Data)
				continue
			}
			q <- evData
		}
	}()
	for {
		ev := <-q
		if err = onEvent(ev); err != nil {
			log.Error().Err(err).Msg("on event")
		}
	}
	if err := evReader.Close(); err != nil {
		log.Error().Err(err).Msg("close event reader")
	}
	return nil
}

func onEvent(evData zoneRecordEvent) error {
	wiper := func() error {
		for {
			err := wipe(evData.Name)
			switch {
			case errors.Is(err, os.ErrDeadlineExceeded):
				log.Error().Err(err).Msg("pdns control socket")
			case err != nil:
				log.Error().Err(err).Msg("wipe error")
			default:
				return nil
			}
		}
	}
	return wiper()
}

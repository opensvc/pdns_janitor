package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"opensvc.com/opensvc/core/client"
	"opensvc.com/opensvc/core/event"
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
	tempSock           string
	connectInterval    time.Duration

	pdnsIsConnected   atomic.Bool
	pdnsConn          net.Conn
	evHandlingTimeout = 300 * time.Millisecond
	logLevel          string

	defaultOSVCSock = "/var/run/lsnr/h2.sock"
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

	flag.StringVar(&osvcSock, "osvc-sock", defaultOSVCSock, "the unix domain socket of the opensvc agent api")
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

	defer func() {
		_ = pdnsConn.Close()
		_ = os.Remove(tempSock)
	}()

	for {
		if err := watch(); err != nil {
			log.Error().Err(err).Msg("watch")
		}
		time.Sleep(connectInterval)
	}
}

func pdnsRedial() error {
	minDialTime := lastDial.Add(connectInterval)
	if time.Now().Before(minDialTime) {
		time.Sleep(minDialTime.Sub(time.Now()))
	}
	lastDial = time.Now()
	if pdnsConn != nil {
		if err := pdnsConn.Close(); err != nil {
			log.Error().Err(err).Msg("redial: pdns connection close")
		}
		pdnsConn = nil
	}
	if tempSock != "" {
		if err := os.Remove(tempSock); errors.Is(err, os.ErrExist) {
			// pass
		} else if err != nil {
			log.Error().Err(err).Msg("redial: pdns client socket file remove")
		}
	}
	if err := pdnsDial(); err != nil {
		return err
	}
	return nil
}

func pdnsDial() error {
	if tempSock == "" {
		tempSockDir := filepath.Dir(pdnsSock)
		if f, err := os.CreateTemp(tempSockDir, filepath.Base(pdnsSock)+".cli."); err != nil {
			return err
		} else {
			tempSock = f.Name()
			f.Close()
			os.Remove(tempSock)
		}
		log.Info().Msgf("client socket created: %s => %s", tempSock, pdnsSock)
	}
	laddr := net.UnixAddr{tempSock, "unixgram"}
	raddr := net.UnixAddr{pdnsSock, "unixgram"}
	if conn, err := net.DialUnix("unixgram", &laddr, &raddr); err != nil {
		return err
	} else {
		log.Info().Msg("pdns recursor connected")
		pdnsConn = conn
		pdnsIsConnected.Store(true)
	}
	return nil
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
	msg := fmt.Sprintf("wipe-cache %s", evData.Name)
	wipe := func() error {
		deadline := time.Now().Add(evHandlingTimeout)
		if err := pdnsConn.SetDeadline(deadline); err != nil {
			return err
		}
		log.Info().Msgf(">>> %s", msg)
		if _, err := pdnsConn.Write([]byte(msg + "\n")); err != nil {
			return err
		}
		var buff [1024]byte
		n, err := pdnsConn.Read(buff[:])
		if err != nil {
			return err
		}
		if n > 0 {
			log.Info().Msgf("<<< %s", string(buff[:n-1]))
		}
		return nil
	}
	wiper := func() error {
		for {
			if !pdnsIsConnected.Load() {
				if err := pdnsRedial(); err != nil {
					return err
				}
			}
			err := wipe()
			switch {
			case errors.Is(err, os.ErrDeadlineExceeded):
				log.Error().Err(err).Msg("pdns control socket")
			case err != nil:
				pdnsIsConnected.Store(false)
			default:
				return nil
			}
		}
	}
	return wiper()
}

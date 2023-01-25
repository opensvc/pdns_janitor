package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"opensvc.com/opensvc/core/client"
	"opensvc.com/opensvc/core/event"
	"opensvc.com/opensvc/daemon/msgbus"
)

var (
	apiUrl = "/var/lib/opensvc/lsnr/h2.sock"
)

func main() {
	delay := 2 * time.Second
	for {
		if err := watch(); err != nil {
			fmt.Printf("watch error %s\n", err)
		}
		fmt.Printf("wait %s before retry...\n", delay)
		time.Sleep(delay)
	}
}

func watch() (err error) {
	var (
		cli      *client.T
		evReader event.ReadCloser
	)

	if cli, err = client.New(client.WithURL(apiUrl), client.WithInsecureSkipVerify(true)); err != nil {
		return errors.Wrap(err, "create client")
	}
	if evReader, err = cli.NewGetEvents().SetFilters([]string{"InstanceStatusUpdated"}).GetReader(); err != nil {
		return errors.Wrap(err, "GetReader")
	}
	defer func() {
		_ = evReader.Close()
	}()

	err = watchEvents(evReader)
	return errors.Wrap(err, "watchEvents")
}

func watchEvents(evReader event.ReadCloser) (err error) {
	var (
		ev *event.Event
	)

	for {
		if ev, err = evReader.Read(); err != nil {
			return err
		}
		if err = showEvent(ev); err != nil {
			return err
		}
	}
}

func showEvent(ev *event.Event) error {
	fmt.Printf("------ read event kind: %s, time: %s\n", ev.Kind, ev.Time)
	if ev.Kind != "InstanceStatusUpdated" {
		return errors.Errorf("unexpected kind: %s", ev.Kind)
	}
	var evData msgbus.InstanceStatusUpdated
	err := json.Unmarshal(ev.Data, &evData)
	if err != nil {
		return errors.Errorf("Unmarshal error %s on '%s'", err, ev.Data)
	}
	resources := evData.Value.Resources
	for i, r := range resources {
		fmt.Printf("path: %s index: %d rid:%s status: %s\n", evData.Path, i, r.Rid, r.Status)
	}
	return nil
}

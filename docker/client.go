package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/crosbymichael/log"
	"github.com/crosbymichael/skydock/utils"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"syscall"
)

type (
	Docker interface {
		FetchAllContainers() ([]*Container, error)
		FetchContainer(name, image string) (*Container, error)
		GetEvents() chan *Event
	}

	Event struct {
		ContainerId string `json:"id"`
		Status      string `json:"status"`
		Image       string `json:"from"`
	}

	ContainerConfig struct {
		Hostname string
		Image    string
		Env      []string
	}

	Binding struct {
		HostIp   string
		HostPort string
	}

	NetworkSettings struct {
		IpAddress string
		Ports     map[string][]Binding
	}

	State struct {
		Running bool
	}

	Container struct {
		Id              string
		Image           string
		Name            string
		Config          *ContainerConfig
		NetworkSettings *NetworkSettings
		State           State
	}

	dockerClient struct {
		path string
	}
)

var (
	ErrImageNotTagged = errors.New("image not tagged")
)

func NewClient(path string) (Docker, error) {
	return &dockerClient{path}, nil
}

func (d *dockerClient) newConn() (*httputil.ClientConn, error) {
	prot, path := utils.SplitURI(d.path)
	conn, err := net.Dial(prot, path)
	if err != nil {
		return nil, err
	}
	return httputil.NewClientConn(conn, nil), nil
}

func (d *dockerClient) FetchContainer(name, image string) (*Container, error) {
	c, err := d.newConn()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	req, err := http.NewRequest("GET", fmt.Sprintf("/containers/%s/json", name), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var (
			container *Container
			d         = json.NewDecoder(resp.Body)
		)

		if err = d.Decode(&container); err != nil {
			return nil, err
		}

		// These should match or else it's from an image that is not tagged
		if image != "" && utils.RemoveTag(image) != utils.RemoveTag(container.Config.Image) {
			return nil, ErrImageNotTagged
		}
		container.Image = image

		return container, nil
	}
	return nil, fmt.Errorf("Could not fetch container %d", resp.StatusCode)
}

func (d *dockerClient) FetchAllContainers() ([]*Container, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("/containers/json"), nil)
	if err != nil {
		return nil, err
	}

	c, err := d.newConn()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var containers []*Container
		if err = json.NewDecoder(resp.Body).Decode(&containers); err != nil {
			return nil, err
		}
		return containers, nil
	}
	return nil, fmt.Errorf("invalid HTTP request %d %s", resp.StatusCode, resp.Status)
}

func (d *dockerClient) GetEvents() chan *Event {
	eventChan := make(chan *Event, 100) // 100 event buffer
	go func() {
		defer close(eventChan)

		c, err := d.newConn()
		if err != nil {
			log.Logf(log.FATAL, "cannot connect to docker: %s", err)
			return
		}
		defer c.Close()

		req, err := http.NewRequest("GET", "/events", nil)
		if err != nil {
			log.Logf(log.ERROR, "bad request for events: %s", err)
			return
		}

		resp, err := c.Do(req)
		if err != nil {
			log.Logf(log.FATAL, "cannot connect to events endpoint: %s", err)
			return
		}
		defer resp.Body.Close()

		// handle signals to stop the socket
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			for sig := range sigChan {
				log.Logf(log.INFO, "received signal '%v', exiting", sig)

				c.Close()
				close(eventChan)
				os.Exit(0)
			}
		}()

		dec := json.NewDecoder(resp.Body)
		for {
			var event *Event
			if err := dec.Decode(&event); err != nil {
				if err == io.EOF {
					break
				}
				log.Logf(log.ERROR, "cannot decode json: %s", err)
				continue
			}
			eventChan <- event
		}
		log.Logf(log.DEBUG, "closing event channel")
	}()
	return eventChan
}

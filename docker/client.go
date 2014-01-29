package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/crosbymichael/skydock/utils"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
)

type (
	Docker interface {
		FetchAllContainers() ([]*Container, error)
		FetchContainer(name, image string) (*Container, error)
		GetEvents() (<-chan *Event, error)
	}

	Event struct {
		ContainerId string `json:"id"`
		Status      string `json:"status"`
		Image       string `json:"from"`
	}

	ContainerConfig struct {
		Hostname     string
		Image        string
		ExposedPorts map[string]struct{}
	}

	NetworkSettings struct {
		IpAddress   string
		PortMapping map[string]map[string]string
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
	conn, err := net.Dial("unix", d.path)
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

func (d *dockerClient) GetEvents() (<-chan *Event, error) {
	c, err := d.newConn()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	eventChan := make(chan *Event, 100) // 100 event buffer

	req, err := http.NewRequest("GET", "/events", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	go func() {
		dec := json.NewDecoder(resp.Body)
		for {
			var event *Event
			if err := dec.Decode(&event); err != nil {
				if err == io.EOF {
					break
				}
				log.Printf("Error decoding json %s\n", err)
				continue
			}
			eventChan <- event
		}
		// Close the event chan then wait for handlers to finish
		close(eventChan)
	}()
	return eventChan, nil
}

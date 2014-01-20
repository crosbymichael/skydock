package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
)

type (
	docker struct {
		c *httputil.ClientConn
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
)

// newClient connects to the unix socket to be used in http requests
func newClient(path string) (*docker, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return &docker{
		c: httputil.NewClientConn(conn, nil),
	}, nil
}

func (d *docker) getEventStream() (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", "/events", nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.c.Do(req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (d *docker) fetchContainer(name, image string) (*Container, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("/containers/%s/json", name), nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.c.Do(req)
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
		if image != "" && removeTag(image) != container.Config.Image {
			return nil, errNotTagged
		}
		container.Image = image

		return container, nil
	}
	return nil, fmt.Errorf("Could not fetch container %d", resp.StatusCode)
}

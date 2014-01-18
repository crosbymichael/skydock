package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/skynetservices/skydns/client"
	"github.com/skynetservices/skydns/msg"
	"io"
	"log"
	"net/http"
	"strings"
)

var (
	dockerUrl      string
	dockerHostName string
	skydnsUrl      string
	secret         string
	ttl            int

	c            *http.Client
	liveServices = make(map[string]*Container)
)

type (
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

	Container struct {
		Id              string
		Image           string
		Name            string
		Config          *ContainerConfig
		NetworkSettings *NetworkSettings
	}
)

func init() {
	flag.StringVar(&dockerUrl, "docker", "", "url to the docker api")
	flag.StringVar(&skydnsUrl, "skydns", "", "url to the skydns url")
	flag.StringVar(&secret, "secret", "", "skydns secret")
	flag.StringVar(&dockerHostName, "hostname", "", "docker host name")
	flag.IntVar(&ttl, "ttl", 60, "default ttl to use when registering a service")
	flag.Parse()
}

func truncate(name string) string {
	return name[:10]
}

func fetchContainer(name, image string) (*Container, error) {
	path := fmt.Sprintf("%s/containers/%s/json", dockerUrl, name)
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		var container *Container
		if err = d.Decode(&container); err != nil {
			return nil, err
		}
		container.Image = image
		return container, nil
	}
	return nil, fmt.Errorf("Could not fetch container %d", resp.StatusCode)
}

func removeTag(name string) string {
	return removeSlash(strings.Split(name, ":")[0])
}

func removeSlash(name string) string {
	return strings.Replace(name, "/", "", -1)
}

func cleanImageImage(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		return removeTag(name)
	}
	return removeTag(parts[1])
}

// <uuid>.<host>.<region>.<version>.<service>.<environment>.skydns.local
func createService(container *Container) *msg.Service {
	return &msg.Service{
		Name:        cleanImageImage(container.Image),
		Version:     removeSlash(container.Name),
		Host:        container.NetworkSettings.IpAddress,
		Environment: dockerHostName,
		TTL:         uint32(ttl), // 60 seconds
		Port:        80,
	}
}

func main() {
	c = &http.Client{}
	skydns, err := client.NewClient(skydnsUrl, secret)
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/events", dockerUrl), nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	log.Printf("Starting run loop...\n")
	for {
		var event *Event
		if err := dec.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		uuid := truncate(event.ContainerId)

		switch event.Status {
		case "die", "stop", "kill":
			log.Printf("Removing %s for %s from skydns\n", uuid, event.Image)
			delete(liveServices, uuid)
			if err := skydns.Delete(event.ContainerId); err != nil {
				log.Fatal(err)
			}
		case "start", "restart":
			log.Printf("Adding %s for %s\n", uuid, event.Image)

			container, err := fetchContainer(event.ContainerId, event.Image)
			if err != nil {
				log.Fatal(err)
			}
			liveServices[uuid] = container
			service := createService(container)

			if err := skydns.Add(uuid, service); err != nil {
				panic(err)
			}
		}
	}
}

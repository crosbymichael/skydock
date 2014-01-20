/*
   TODO: Add restore/reconnect logic
   Multihost
   Multiple ports
*/

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/skynetservices/skydns/client"
	"github.com/skynetservices/skydns/msg"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

var (
	dockerPath     string
	dockerHostName string
	environment    string
	skydnsUrl      string
	secret         string
	ttl            int
	beat           int

	skydns  *client.Client
	running = make(map[string]struct{})
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

func init() {
	flag.StringVar(&dockerPath, "s", "/var/run/docker.sock", "path to the docker unix socket")
	flag.StringVar(&skydnsUrl, "skydns", "", "url to the skydns url")
	flag.StringVar(&secret, "secret", "", "skydns secret")
	flag.StringVar(&dockerHostName, "hostname", "docker", "docker host name")
	flag.StringVar(&environment, "environment", "dev", "environment name where service is running")
	flag.IntVar(&ttl, "ttl", 60, "default ttl to use when registering a service")
	flag.IntVar(&beat, "beat", 0, "heartbeat interval")
	flag.Parse()

	if beat < 1 {
		beat = ttl - (ttl / 4)
	}

	if skydnsUrl == "" {
		skydnsUrl = "http://" + os.Getenv("SKYDNS_PORT_8080_TCP_ADDR") + ":8080"
	}
}

func newClient(path string) (*httputil.ClientConn, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return httputil.NewClientConn(conn, nil), nil
}

func truncate(name string) string {
	return name[:10]
}

func fetchContainer(name, image string) (*Container, error) {
	path := fmt.Sprintf("/containers/%s/json", name)
	c, err := newClient(dockerPath)
	if err != nil {
		return nil, err
	}
	defer c.Close()

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

func heartbeat(uuid string) {
	if _, exists := running[uuid]; exists {
		return
	}
	running[uuid] = struct{}{}
	defer delete(running, uuid)

	for _ = range time.Tick(time.Duration(beat) * time.Second) {
		container, err := fetchContainer(uuid, "")
		if err != nil {
			log.Println(err)
			break
		}

		if container.State.Running {
			log.Printf("Updating ttl for %s\n", container.Name)

			if err := skydns.Update(uuid, uint32(ttl)); err != nil {
				log.Println(err)
				break
			}
		} else {
			if err := skydns.Delete(uuid); err != nil {
				log.Println(err)
				break
			}
		}
	}
}

func restoreContainers() {
	path := fmt.Sprintf("/containers/json")
	c, err := newClient(dockerPath)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		var containers []*Container
		if err = d.Decode(&containers); err != nil {
			log.Println(err)
		}
		for _, cnt := range containers {
			container, err := fetchContainer(cnt.Id, cnt.Image)
			if err != nil {
				log.Println(err)
			}
			service := createService(container)
			uuid := truncate(cnt.Id)
			log.Println(fmt.Sprintf("Adding %v (%v)", uuid, cleanImageImage(cnt.Image)))
			if err := skydns.Add(uuid, service); err != nil {
				log.Fatal(err)
			}
			go heartbeat(uuid)
		}
	}
}

// <uuid>.<host>.<region>.<version>.<service>.<environment>.skydns.local
func createService(container *Container) *msg.Service {
	return &msg.Service{
		Name:        cleanImageImage(container.Image), // Service name
		Version:     removeSlash(container.Name),      // Instance of the service
		Host:        container.NetworkSettings.IpAddress,
		Environment: environment,    // testing, prod, dev
		Region:      dockerHostName, // Docker host where it's running
		TTL:         uint32(ttl),    // 60 seconds
		Port:        80,             // TODO: How to handle multiple ports
	}
}

func main() {
	c, err := newClient(dockerPath)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	skydns, err = client.NewClient(skydnsUrl, secret, dockerHostName, 53)
	if err != nil {
		log.Fatal(err)
	}

	restoreContainers()
	req, err := http.NewRequest("GET", "/events", nil)
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

			if err := skydns.Delete(uuid); err != nil {
				log.Fatal(err)
			}
		case "start", "restart":
			log.Printf("Adding %s for %s\n", uuid, event.Image)

			container, err := fetchContainer(event.ContainerId, event.Image)
			if err != nil {
				log.Fatal(err)
			}
			service := createService(container)

			if err := skydns.Add(uuid, service); err != nil {
				log.Fatal(err)
			}
			go heartbeat(uuid)
		}
	}
}

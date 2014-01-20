/*
   Multihost
   Multiple ports
*/

package main

import (
	"encoding/json"
	"errors"
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
	"time"
)

var (
	pathToSocket string
	domain       string
	environment  string
	skydnsUrl    string
	secret       string
	ttl          int
	beat         int

	skydns       *client.Client
	running      = make(map[string]struct{})
	errNotTagged = errors.New("image not tagged")
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
	flag.StringVar(&pathToSocket, "s", "/var/run/docker.sock", "path to the docker unix socket")
	flag.StringVar(&skydnsUrl, "skydns", "", "url to the skydns url")
	flag.StringVar(&secret, "secret", "", "skydns secret")
	flag.StringVar(&domain, "domain", "", "same domain passed to skydns")
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

	if domain == "" {
		log.Fatal("Must specify your skydns domain")
	}
}

// newClient connects to the unix socket to be used in http requests
func newClient(path string) (*httputil.ClientConn, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return httputil.NewClientConn(conn, nil), nil
}

func fetchContainer(name, image string) (*Container, error) {
	c, err := newClient(pathToSocket)
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
		if image != "" && removeTag(image) != container.Config.Image {
			return nil, errNotTagged
		}
		container.Image = image

		return container, nil
	}
	return nil, fmt.Errorf("Could not fetch container %d", resp.StatusCode)
}

func heartbeat(uuid string) {
	if _, exists := running[uuid]; exists {
		return
	}
	// TODO: not safe for concurrent access
	running[uuid] = struct{}{}
	defer delete(running, uuid)

	var (
		errorCount int
		err        error
		container  *Container
	)

	for _ = range time.Tick(time.Duration(beat) * time.Second) {
		if errorCount > 10 {
			// if we encountered more than 10 errors just quit
			log.Printf("Aborting heartbeat for %s after 10 errors\n", uuid)
			return
		}

		if container, err = fetchContainer(uuid, ""); err != nil {
			errorCount++
			log.Println(err)
			break
		}

		if !container.State.Running {
			if err := skydns.Delete(uuid); err != nil {
				log.Println(err)
			}
			return
		}

		// don't fill logs if we have a low beat
		// may need to do something better here
		if beat >= 30 {
			log.Printf("Updating ttl for %s\n", container.Name)
		}

		if err := skydns.Update(uuid, uint32(ttl)); err != nil {
			errorCount++
			log.Println(err)
			break
		}
	}
}

// restoreContainers loads all running containers and inserts
// them into skydns when skydock starts
func restoreContainers(c *httputil.ClientConn) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("/containers/json"), nil)
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var (
			containers []*Container
			container  *Container
			d          = json.NewDecoder(resp.Body)
		)

		if err = d.Decode(&containers); err != nil {
			return err
		}

		for _, cnt := range containers {
			uuid := truncate(cnt.Id)
			if container, err = fetchContainer(uuid, cnt.Image); err != nil {
				if err != errNotTagged {
					log.Printf("Failed to fetch %s for restore - %s\n", cnt.Id, err)
				}
				continue
			}

			if err := sendService(uuid, createService(container)); err != nil {
				log.Printf("Failed to send %s to skydns for restore - %s\n", uuid, err)
			}
		}
	}
	return nil
}

// <uuid>.<host>.<region>.<version>.<service>.<environment>.skydns.local
func createService(container *Container) *msg.Service {
	return &msg.Service{
		Name:        cleanImageImage(container.Image), // Service name
		Version:     removeSlash(container.Name),      // Instance of the service
		Host:        container.NetworkSettings.IpAddress,
		Environment: environment, // testing, prod, dev
		TTL:         uint32(ttl), // 60 seconds
		Port:        80,          // TODO: How to handle multiple ports
	}
}

// sendService sends the uuid and service data to skydns
func sendService(uuid string, service *msg.Service) error {
	log.Printf("Adding %s (%s)\n", uuid, service.Name)

	if err := skydns.Add(uuid, service); err != nil {
		return err
	}
	go heartbeat(uuid)

	return nil
}

func main() {
	c, err := newClient(pathToSocket)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	skydns, err = client.NewClient(skydnsUrl, secret, domain, 53)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Starting restore of running containers...")
	if err := restoreContainers(c); err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("GET", "/events", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	log.Println("Starting run loop...")

	d := json.NewDecoder(resp.Body)
	for {
		var event *Event
		if err := d.Decode(&event); err != nil {
			if err == io.EOF {
				log.Println("Stopping cleanly via EOF")
				break
			}
			log.Printf("Error decoding json %s\n", err)
		}
		uuid := truncate(event.ContainerId)

		switch event.Status {
		case "die", "stop", "kill":
			log.Printf("Removing %s for %s from skydns\n", uuid, event.Image)

			if err := skydns.Delete(uuid); err != nil {
				log.Printf("Error deleting %s - %s\n", uuid, err)
			}
		case "start", "restart":
			log.Printf("Adding %s for %s\n", uuid, event.Image)

			container, err := fetchContainer(uuid, event.Image)
			if err != nil {
				if err != errNotTagged {
					log.Printf("Error fetching container %s\n", err)
				}
				continue
			}

			if err := sendService(uuid, createService(container)); err != nil {
				log.Printf("Error sending %s to skydns - %s\n", uuid, err)
			}
		}
	}
}

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
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	pathToSocket     string
	domain           string
	environment      string
	skydnsUrl        string
	secret           string
	ttl              int
	beat             int
	numberOfHandlers int

	skydns       *client.Client
	dockerClient *docker
	running      = make(map[string]struct{})
	errNotTagged = errors.New("image not tagged")
)

func init() {
	flag.StringVar(&pathToSocket, "s", "/var/run/docker.sock", "path to the docker unix socket")
	flag.StringVar(&skydnsUrl, "skydns", "", "url to the skydns url")
	flag.StringVar(&secret, "secret", "", "skydns secret")
	flag.StringVar(&domain, "domain", "", "same domain passed to skydns")
	flag.StringVar(&environment, "environment", "dev", "environment name where service is running")
	flag.IntVar(&ttl, "ttl", 60, "default ttl to use when registering a service")
	flag.IntVar(&beat, "beat", 0, "heartbeat interval")
	flag.IntVar(&numberOfHandlers, "workers", 10, "number of concurrent workers")

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

		if container, err = dockerClient.fetchContainer(uuid, ""); err != nil {
			errorCount++
			log.Println(err)
			break
		}

		if !container.State.Running {
			if err := removeService(uuid); err != nil {
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
func restoreContainers() error {
	req, err := http.NewRequest("GET", fmt.Sprintf("/containers/json"), nil)
	if err != nil {
		return err
	}

	c, err := dockerClient.newConn()
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var (
			containers []*Container
			container  *Container
		)

		if err = json.NewDecoder(resp.Body).Decode(&containers); err != nil {
			return err
		}

		for _, cnt := range containers {
			uuid := truncate(cnt.Id)
			if container, err = dockerClient.fetchContainer(uuid, cnt.Image); err != nil {
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

func removeService(uuid string) error {
	log.Printf("Removing %s from skydns\n", uuid)
	return skydns.Delete(uuid)
}

func addService(uuid, image string) error {
	log.Printf("Adding %s for %s\n", uuid, image)
	container, err := dockerClient.fetchContainer(uuid, image)
	if err != nil {
		if err != errNotTagged {
			return err
		}
		return nil
	}

	if err := sendService(uuid, createService(container)); err != nil {
		return err
	}
	return nil
}

func eventHandler(c chan *Event, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()

	for event := range c {
		uuid := truncate(event.ContainerId)
		switch event.Status {
		case "die", "stop", "kill":
			if err := removeService(uuid); err != nil {
				log.Printf("Error deleting %s - %s\n", uuid, err)
			}
		case "start", "restart":
			if err := addService(uuid, event.Image); err != nil {
				log.Printf("Error adding %s - %s\n", uuid, err)
			}
		}
	}
}

func main() {
	var (
		err       error
		eventChan = make(chan *Event, 100) // 100 event buffer
		group     = &sync.WaitGroup{}
	)
	if dockerClient, err = newClient(pathToSocket); err != nil {
		log.Fatal(err)
	}

	if skydns, err = client.NewClient(skydnsUrl, secret, domain, 53); err != nil {
		log.Fatal(err)
	}

	log.Println("Starting restore of running containers...")
	if err := restoreContainers(); err != nil {
		log.Fatal(err)
	}

	c, err := dockerClient.newConn()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// Start event handlers
	for i := 0; i < numberOfHandlers; i++ {
		go eventHandler(eventChan, group)
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
				break
			}
			log.Printf("Error decoding json %s\n", err)
			continue
		}
		eventChan <- event
	}

	// Close the event chan then wait for handlers to finish
	close(eventChan)
	group.Wait()

	log.Println("Stopping cleanly via EOF")
}

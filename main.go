/*
   Multihost
   Multiple ports
*/

package main

import (
	"flag"
	"github.com/crosbymichael/skydock/docker"
	"github.com/crosbymichael/skydock/utils"
	"github.com/skynetservices/skydns/client"
	"github.com/skynetservices/skydns/msg"
	"log"
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

	skydns       Skydns
	dockerClient docker.Docker
	running      = make(map[string]struct{})
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
}

func validateSettings() {
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
		container  *docker.Container
	)

	for _ = range time.Tick(time.Duration(beat) * time.Second) {
		if errorCount > 10 {
			// if we encountered more than 10 errors just quit
			log.Printf("Aborting heartbeat for %s after 10 errors\n", uuid)
			return
		}

		if container, err = dockerClient.FetchContainer(uuid, ""); err != nil {
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
	containers, err := dockerClient.FetchAllContainers()
	if err != nil {
		return err
	}

	var container *docker.Container
	for _, cnt := range containers {
		uuid := utils.Truncate(cnt.Id)
		if container, err = dockerClient.FetchContainer(uuid, cnt.Image); err != nil {
			if err != docker.ErrImageNotTagged {
				log.Printf("Failed to fetch %s for restore - %s\n", cnt.Id, err)
			}
			continue
		}

		if err := sendService(uuid, createService(container)); err != nil {
			log.Printf("Failed to send %s to skydns for restore - %s\n", uuid, err)
		}
	}
	return nil
}

// <uuid>.<host>.<region>.<version>.<service>.<environment>.skydns.local
func createService(container *docker.Container) *msg.Service {
	return &msg.Service{
		Name:        utils.CleanImageImage(container.Image), // Service name
		Version:     utils.RemoveSlash(container.Name),      // Instance of the service
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

	container, err := dockerClient.FetchContainer(uuid, image)
	if err != nil {
		if err != docker.ErrImageNotTagged {
			return err
		}
		return nil
	}

	if err := sendService(uuid, createService(container)); err != nil {
		return err
	}
	return nil
}

func eventHandler(c <-chan *docker.Event, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()

	for event := range c {
		uuid := utils.Truncate(event.ContainerId)

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
	validateSettings()

	var (
		err   error
		group = &sync.WaitGroup{}
	)

	if dockerClient, err = docker.NewClient(pathToSocket); err != nil {
		log.Fatal(err)
	}

	if skydns, err = client.NewClient(skydnsUrl, secret, domain, "172.17.42.1:53"); err != nil {
		log.Fatal(err)
	}

	log.Println("Starting restore of running containers...")
	if err := restoreContainers(); err != nil {
		log.Fatal(err)
	}

	events, err := dockerClient.GetEvents()
	if err != nil {
		log.Fatal(err)
	}

	// Start event handlers
	for i := 0; i < numberOfHandlers; i++ {
		go eventHandler(events, group)
	}

	log.Println("Starting run loop...")
	group.Wait()
	log.Println("Stopping cleanly via EOF")
}

/*
   Multihost
   Multiple ports
*/

package main

import (
	"flag"
	"fmt"
	"github.com/crosbymichael/log"
	"github.com/crosbymichael/skydock/docker"
	"github.com/crosbymichael/skydock/utils"
	influxdb "github.com/influxdb/influxdb-go"
	"github.com/skynetservices/skydns/client"
	"github.com/skynetservices/skydns/msg"
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
		fatal(fmt.Errorf("Must specify your skydns domain"))
	}
}

func setupLogger() error {
	var (
		logger log.Logger
		err    error
	)

	if host := os.Getenv("INFLUXDB_HOST"); host != "" {
		config := &influxdb.ClientConfig{
			Host:     host,
			Database: os.Getenv("INFLUXDB_DATABASE"),
			Username: os.Getenv("INFLUXDB_USER"),
			Password: os.Getenv("INFLUXDB_PASSWORD"),
		}

		logger, err = log.NewInfluxdbLogger(fmt.Sprintf("%s.%s", environment, domain), "skydock", config)
		if err != nil {
			return err
		}
	} else {
		logger = log.NewStandardLevelLogger("skydock")
	}

	if err := log.SetLogger(logger); err != nil {
		return err
	}
	return nil
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
			log.Logf(log.ERROR, "aborting heartbeat for %s after 10 errors", uuid)
			return
		}

		if container, err = dockerClient.FetchContainer(uuid, ""); err != nil {
			errorCount++
			log.Logf(log.ERROR, "%s", err)
			break
		}

		if !container.State.Running {
			if err := removeService(uuid); err != nil {
				log.Logf(log.ERROR, "%s", err)
			}
			return
		}

		// don't fill logs if we have a low beat
		// may need to do something better here
		if beat >= 30 {
			log.Logf(log.INFO, "updating ttl for %s", container.Name)
		}

		if err := updateService(uuid, ttl); err != nil {
			errorCount++
			log.Logf(log.ERROR, "%s", err)
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
				log.Logf(log.ERROR, "failed to fetch %s on restore: %s", cnt.Id, err)
			}
			continue
		}

		if err := sendService(uuid, createService(container)); err != nil {
			log.Logf(log.ERROR, "failed to send %s to skydns on restore: %s", uuid, err)
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
	log.Logf(log.INFO, "adding %s (%s) to skydns", uuid, service.Name)
	if err := skydns.Add(uuid, service); err != nil {
		// ignore erros for conflicting uuids and start the heartbeat again
		if err != client.ErrConflictingUUID {
			return err
		}
		log.Logf(log.INFO, "service already exists for %s. Resetting ttl.", uuid)
		updateService(uuid, ttl)
	}
	go heartbeat(uuid)
	return nil
}

func removeService(uuid string) error {
	log.Logf(log.INFO, "removing %s from skydns", uuid)
	return skydns.Delete(uuid)
}

func addService(uuid, image string) error {
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

func updateService(uuid string, ttl int) error {
	return skydns.Update(uuid, uint32(ttl))
}

func eventHandler(c <-chan *docker.Event, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()

	for event := range c {
		uuid := utils.Truncate(event.ContainerId)

		switch event.Status {
		case "die", "stop", "kill":
			if err := removeService(uuid); err != nil {
				log.Logf(log.ERROR, "error removing %s from skydns: %s", uuid, err)
			}
		case "start", "restart":
			if err := addService(uuid, event.Image); err != nil {
				log.Logf(log.ERROR, "error adding %s to skydns: %s", uuid, err)
			}
		}
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%s", err)
	os.Exit(1)

}

func main() {
	validateSettings()
	if err := setupLogger(); err != nil {
		fatal(err)
	}

	var (
		err   error
		group = &sync.WaitGroup{}
	)

	if dockerClient, err = docker.NewClient(pathToSocket); err != nil {
		log.Logf(log.FATAL, "error connecting to docker: %s", err)
		fatal(err)
	}

	if skydns, err = client.NewClient(skydnsUrl, secret, domain, "172.17.42.1:53"); err != nil {
		log.Logf(log.FATAL, "error connecting to skydns: %s", err)
		fatal(err)
	}

	log.Logf(log.DEBUG, "starting restore of containers")
	if err := restoreContainers(); err != nil {
		log.Logf(log.FATAL, "error restoring containers: %s", err)
		fatal(err)
	}

	events, err := dockerClient.GetEvents()
	if err != nil {
		log.Logf(log.FATAL, "error connecting to events endpoint: %s", err)
		fatal(err)
	}

	// Start event handlers
	for i := 0; i < numberOfHandlers; i++ {
		go eventHandler(events, group)
	}

	log.Logf(log.DEBUG, "starting main process")
	group.Wait()
	log.Logf(log.DEBUG, "stopping cleanly via EOF")
}

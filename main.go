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
	pluginFile       string

	skydns       Skydns
	dockerClient docker.Docker
	plugins      *pluginRuntime
	running      = make(map[string]struct{})
	runningLock  = sync.Mutex{}
)

func init() {
	flag.StringVar(&pathToSocket, "s", "/var/run/docker.sock", "path to the docker unix socket")
	flag.StringVar(&skydnsUrl, "skydns", "", "url to the skydns url")
	flag.StringVar(&secret, "secret", "", "skydns secret")
	flag.StringVar(&domain, "domain", "", "same domain passed to skydns")
	flag.StringVar(&environment, "environment", "dev", "environment name where service is running")
	flag.IntVar(&ttl, "ttl", 60, "default ttl to use when registering a service")
	flag.IntVar(&beat, "beat", 0, "heartbeat interval")
	flag.IntVar(&numberOfHandlers, "workers", 3, "number of concurrent workers")
	flag.StringVar(&pluginFile, "plugins", "/plugins/default.js", "file containing javascript plugins (plugins.js)")

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
	runningLock.Lock()
	if _, exists := running[uuid]; exists {
		runningLock.Unlock()
		return
	}
	running[uuid] = struct{}{}
	runningLock.Unlock()

	defer func() {
		runningLock.Lock()
		delete(running, uuid)
		runningLock.Unlock()
	}()

	var errorCount int
	for _ = range time.Tick(time.Duration(beat) * time.Second) {
		if errorCount > 10 {
			// if we encountered more than 10 errors just quit
			log.Logf(log.ERROR, "aborting heartbeat for %s after 10 errors", uuid)
			return
		}

		// don't fill logs if we have a low beat
		// may need to do something better here
		if beat >= 30 {
			log.Logf(log.INFO, "updating ttl for %s", uuid)
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

		service, err := plugins.createService(container)
		if err != nil {
			// doing a fatal here because we cannot do much if the plugins
			// return an invalid service or error
			fatal(err)
		}
		if err := sendService(uuid, service); err != nil {
			log.Logf(log.ERROR, "failed to send %s to skydns on restore: %s", uuid, err)
		}
	}
	return nil
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

	service, err := plugins.createService(container)
	if err != nil {
		// doing a fatal here because we cannot do much if the plugins
		// return an invalid service or error
		fatal(err)
	}

	if err := sendService(uuid, service); err != nil {
		return err
	}
	return nil
}

func updateService(uuid string, ttl int) error {
	return skydns.Update(uuid, uint32(ttl))
}

func eventHandler(c chan *docker.Event, group *sync.WaitGroup) {
	defer group.Done()

	for event := range c {
		log.Logf(log.DEBUG, "received event (%s) %s %s", event.Status, event.ContainerId, event.Image)
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
	fmt.Fprintf(os.Stderr, "%s\n", err)
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

	plugins, err = newRuntime(pluginFile)
	if err != nil {
		fatal(err)
	}

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

	events := dockerClient.GetEvents()

	group.Add(numberOfHandlers)
	// Start event handlers
	for i := 0; i < numberOfHandlers; i++ {
		go eventHandler(events, group)
	}

	log.Logf(log.DEBUG, "starting main process")
	group.Wait()
	log.Logf(log.DEBUG, "stopping cleanly via EOF")
}

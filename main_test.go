package main

import (
	"fmt"
	"github.com/crosbymichael/skydock/docker"
	"github.com/skynetservices/skydns/client"
	"github.com/skynetservices/skydns/msg"
	"sync"
	"testing"
	"time"
)

type mockSkydns struct {
	services map[string]*msg.Service
}

func (s *mockSkydns) Add(uuid string, service *msg.Service) error {
	if _, exists := s.services[uuid]; exists {
		return client.ErrConflictingUUID
	}
	s.services[uuid] = service

	return nil
}

func (s *mockSkydns) Update(uuid string, ttl uint32) error {
	if _, exists := s.services[uuid]; !exists {
		return client.ErrServiceNotFound
	}
	s.services[uuid].TTL = ttl

	return nil
}

func (s *mockSkydns) Delete(uuid string) error {
	if _, exists := s.services[uuid]; !exists {
		return client.ErrServiceNotFound
	}
	delete(s.services, uuid)

	return nil
}

type mockDocker struct {
	containers map[string]*docker.Container
}

func (d *mockDocker) FetchContainer(name, image string) (*docker.Container, error) {
	if _, exists := d.containers[name]; !exists {
		return nil, fmt.Errorf("container not exists")
	}
	return d.containers[name], nil
}

func (d *mockDocker) FetchAllContainers() ([]*docker.Container, error) {
	out := make([]*docker.Container, len(d.containers))

	i := 0
	for _, v := range d.containers {
		out[i] = v
		i++
	}
	return out, nil
}

func (d *mockDocker) GetEvents() chan *docker.Event {
	return nil
}

func TestCreateService(t *testing.T) {
	environment = "production"
	ttl = 30

	p, err := newRuntime("")
	if err != nil {
		t.Fatal(err)
	}
	plugins = p

	container := &docker.Container{
		Image: "crosbymichael/redis:latest",
		Name:  "redis1",
		NetworkSettings: &docker.NetworkSettings{
			IpAddress: "192.168.1.10",
		},
	}

	service, err := p.createService(container)
	if err != nil {
		t.Fatal(err)
	}

	if service.Version != "redis1" {
		t.Fatalf("Expected version redis1 got %s", service.Version)
	}

	if service.Host != "192.168.1.10" {
		t.Fatalf("Expected host 192.168.1.10 got %s", service.Host)
	}

	if service.TTL != uint32(30) {
		t.Fatalf("Expected ttl 30 got %d", service.TTL)
	}

	if service.Environment != "production" {
		t.Fatalf("Expected environment production got %s", service.Environment)
	}

	if service.Name != "redis" {
		t.Fatalf("Expected name redis got %s", service.Name)
	}
}

func TestAddService(t *testing.T) {
	p, err := newRuntime("")
	if err != nil {
		t.Fatal(err)
	}
	plugins = p

	skydns = &mockSkydns{make(map[string]*msg.Service)}
	dockerClient = &mockDocker{
		containers: map[string]*docker.Container{
			"1": {
				Image: "crosbymichael/redis:latest",
				Name:  "redis1",
				NetworkSettings: &docker.NetworkSettings{
					IpAddress: "192.168.1.10",
				},
			},
		},
	}

	if err := addService("1", "crosbymichael/redis"); err != nil {
		t.Fatal(err)
	}

	service := skydns.(*mockSkydns).services["1"]

	if service.Version != "redis1" {
		t.Fatalf("Expected version redis1 got %s", service.Version)
	}

	if service.Host != "192.168.1.10" {
		t.Fatalf("Expected host 192.168.1.10 got %s", service.Host)
	}

	if service.TTL != uint32(30) {
		t.Fatalf("Expected ttl 30 got %d", service.TTL)
	}

	if service.Environment != "production" {
		t.Fatalf("Expected environment production got %s", service.Environment)
	}

	if service.Name != "redis" {
		t.Fatalf("Expected name redis got %s", service.Name)
	}
}

func TestRemoveService(t *testing.T) {
	p, err := newRuntime("")
	if err != nil {
		t.Fatal(err)
	}
	plugins = p

	skydns = &mockSkydns{make(map[string]*msg.Service)}
	dockerClient = &mockDocker{
		containers: map[string]*docker.Container{
			"1": {
				Image: "crosbymichael/redis:latest",
				Name:  "redis1",
				NetworkSettings: &docker.NetworkSettings{
					IpAddress: "192.168.1.10",
				},
			},
		},
	}

	if err := addService("1", "crosbymichael/redis"); err != nil {
		t.Fatal(err)
	}

	service := skydns.(*mockSkydns).services["1"]

	if service == nil {
		t.Fatalf("Service not properly added")
	}

	if err := removeService("1"); err != nil {
		t.Fatal(err)
	}

	service = skydns.(*mockSkydns).services["1"]

	if service != nil {
		t.Fatalf("Service not properly removed")
	}
}

func TestEventHandler(t *testing.T) {
	var (
		events = make(chan *docker.Event)
		group  = &sync.WaitGroup{}
	)

	p, err := newRuntime("")
	if err != nil {
		t.Fatal(err)
	}
	plugins = p

	skydns = &mockSkydns{make(map[string]*msg.Service)}
	container := &docker.Container{
		Image: "crosbymichael/redis:latest",
		Name:  "redis1",
		NetworkSettings: &docker.NetworkSettings{
			IpAddress: "192.168.1.10",
		},
		State: docker.State{
			Running: true,
		},
	}

	dockerClient = &mockDocker{
		containers: map[string]*docker.Container{
			"3": container,
		},
	}

	group.Add(1)
	go eventHandler(events, group)

	events <- &docker.Event{
		Status:      "start",
		Image:       "crosbymichael/redis",
		ContainerId: "3",
	}

	close(events)
	time.Sleep(3 * time.Second)

	service := skydns.(*mockSkydns).services["3"]

	if service == nil {
		t.Fatal("No service added on event")
	}

	group.Wait()
}

package main

import (
	"github.com/crosbymichael/skydock/docker"
	"testing"
)

func TestCreateService(t *testing.T) {
	environment = "production"
	ttl = 30

	container := &docker.Container{
		Image: "crosbymichael/redis:latest",
		Name:  "redis1",
		NetworkSettings: &docker.NetworkSettings{
			IpAddress: "192.168.1.10",
		},
	}

	service := createService(container)

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

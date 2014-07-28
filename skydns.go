package main

import (
	"github.com/skynetservices/skydns1/msg"
)

// Interface to allow mocking of the
// skydns client
type Skydns interface {
	Add(uuid string, service *msg.Service) error
	Delete(uuid string) error
	Update(uuid string, ttl uint32) error
}

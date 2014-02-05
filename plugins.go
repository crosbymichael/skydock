package main

import (
	"fmt"
	"github.com/crosbymichael/log"
	"github.com/crosbymichael/skydock/docker"
	"github.com/crosbymichael/skydock/utils"
	"github.com/robertkrimen/otto"
	"github.com/skynetservices/skydns/msg"
	"io/ioutil"
)

/*
	return &msg.Service{
		Name:        utils.CleanImageImage(container.Image), // Service name
		Version:     utils.RemoveSlash(container.Name),      // Instance of the service
		Host:        container.NetworkSettings.IpAddress,
		Environment: environment, // testing, prod, dev
		TTL:         uint32(ttl), // 60 seconds
		Port:        80,          // TODO: How to handle multiple ports
	}
*/
const defaultCreateService = `
    function createService(container) {
        return {
            Port: 80,
            Environment: defaultEnvironment,
            TTL: defaultTTL,
            Service: cleanImageName(container.Image),
            Instance: removeSlash(container.Name),
            Host: container.NetworkSettings.IpAddress
        }; 
    }
`

type pluginRuntime struct {
	o *otto.Otto
}

func (r *pluginRuntime) createService(container *docker.Container) (*msg.Service, error) {
	value, err := r.o.ToValue(*container)
	if err != nil {
		return nil, err
	}

	result, err := r.o.Call("createService", nil, value)
	if err != nil {
		return nil, err
	}

	if !result.IsObject() {
		return nil, fmt.Errorf("createService plugin did not return a valid object")
	}

	var (
		obj     = result.Object()
		service = &msg.Service{}
	)

	rawTTL, err := getInt(obj, "TTL")
	if err != nil {
		return nil, err
	}

	rawPort, err := getInt(obj, "Port")
	if err != nil {
		return nil, err
	}

	if service.Name, err = getString(obj, "Service"); err != nil {
		return nil, err
	}
	if service.Version, err = getString(obj, "Instance"); err != nil {
		return nil, err
	}
	if service.Host, err = getString(obj, "Host"); err != nil {
		return nil, err
	}
	if service.Environment, err = getString(obj, "Environment"); err != nil {
		return nil, err
	}
	service.TTL = uint32(rawTTL)
	service.Port = uint16(rawPort)

	// I'm glad that is over
	return service, nil
}

func newRuntime(file string) (*pluginRuntime, error) {
	runtime := otto.New()
	if file != "" {
		log.Logf(log.INFO, "loading plugins from %s", file)

		content, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}

		if _, err := runtime.Run(string(content)); err != nil {
			return nil, err
		}
	} else {
		if _, err := runtime.Run(defaultCreateService); err != nil {
			return nil, err
		}
	}

	if err := loadDefaults(runtime); err != nil {
		return nil, err
	}
	return &pluginRuntime{runtime}, nil
}

func loadDefaults(runtime *otto.Otto) error {
	if err := runtime.Set("defaultTTL", ttl); err != nil {
		return err
	}
	if err := runtime.Set("defaultEnvironment", environment); err != nil {
		return err
	}
	if err := runtime.Set("cleanImageName", func(call otto.FunctionCall) otto.Value {
		name := call.Argument(0).String()
		result, _ := otto.ToValue(utils.CleanImageName(name))
		return result
	}); err != nil {
		return err
	}
	if err := runtime.Set("removeSlash", func(call otto.FunctionCall) otto.Value {
		name := call.Argument(0).String()
		result, _ := otto.ToValue(utils.RemoveSlash(name))
		return result
	}); err != nil {
		return err
	}
	return nil
}

// util functions

func getString(obj *otto.Object, name string) (string, error) {
	v, err := obj.Get(name)
	if err != nil {
		return "", err
	}
	return v.ToString()
}

func getInt(obj *otto.Object, name string) (int64, error) {
	v, err := obj.Get(name)
	if err != nil {
		return -1, err
	}
	return v.ToInteger()
}

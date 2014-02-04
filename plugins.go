package main

import (
	"fmt"
	"github.com/crosbymichael/log"
	"github.com/crosbymichael/skydock/docker"
	"github.com/crosbymichael/skydock/utils"
	"github.com/robertkrimen/otto"
	"github.com/skynetservices/skydns/msg"
	"io/ioutil"
	"path"
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
	value = value

	result, err := r.o.Call("createService", nil, value)
	if err != nil {
		panic(err)
		return nil, err
	}

	if !result.IsObject() {
		return nil, fmt.Errorf("createService plugin did not return a valid object")
	}

	obj := result.Object()

	service := &msg.Service{}
	nameValue, err := obj.Get("Service")
	if err != nil {
		panic(err)
		return nil, err
	}
	versionValue, err := obj.Get("Instance")
	if err != nil {
		return nil, err
	}
	hostValue, err := obj.Get("Host")
	if err != nil {
		return nil, err
	}
	envValue, err := obj.Get("Environment")
	if err != nil {
		return nil, err
	}
	ttlValue, err := obj.Get("TTL")
	if err != nil {
		return nil, err
	}
	portValue, err := obj.Get("Port")
	if err != nil {
		return nil, err
	}

	if service.Name, err = nameValue.ToString(); err != nil {
		return nil, err
	}
	if service.Version, err = versionValue.ToString(); err != nil {
		return nil, err
	}
	if service.Host, err = hostValue.ToString(); err != nil {
		return nil, err
	}
	if service.Environment, err = envValue.ToString(); err != nil {
		return nil, err
	}
	rawTTL, err := ttlValue.ToInteger()
	if err != nil {
		return nil, err
	}
	service.TTL = uint32(rawTTL)
	rawPort, err := portValue.ToInteger()
	if err != nil {
		return nil, err
	}
	service.Port = uint16(rawPort)

	// I'm glad that is over
	return service, nil
}

func newRuntime(root string) (*pluginRuntime, error) {
	runtime := otto.New()
	if root != "" {
		dir := path.Join(root, "plugins")
		log.Logf(log.INFO, "loading plugins from %s", dir)

		files, err := ioutil.ReadDir(dir)
		if err != nil {
			return nil, err
		}

		for _, fi := range files {
			log.Logf(log.INFO, "loading plugin %s", fi.Name())

			content, err := ioutil.ReadFile(path.Join(dir, fi.Name()))
			if err != nil {
				return nil, err
			}

			if _, err := runtime.Run(string(content)); err != nil {
				return nil, err
			}
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

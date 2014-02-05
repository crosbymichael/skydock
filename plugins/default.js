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


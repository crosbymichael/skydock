function createService(container) {
    var port = getDefaultPort(container);
    return {
        Port: port,
        Environment: defaultEnvironment,
        TTL: defaultTTL,
        Service: cleanImageName(container.Image),
        Instance: removeSlash(container.Name),
        Host: container.NetworkSettings.IpAddress
    }; 
}

function getDefaultPort(container) {
    // if we have any exposed ports use those
    var port = 0;
    var ports = container.NetworkSettings.Ports;
    if (Object.keys(ports).length > 0) {
        for (var key in ports) {
            var value = ports[key];
            if (value !== null && value.length > 0) {
                for (var i = 0; i < value.length; i++) {
                    var hp = parseInt(value[i].HostPort);
                    if (port === 0 || hp < port) {
                        port = hp; 
                    }
                }  
            } else if (port === 0) {
                // just grab the key value 
                var expose = parseInt(key.split("/")[0]); 
                port = expose;
            }
        }
    } 
     
    if (port === 0) {
        port = 80; 
    }
    return port;
}

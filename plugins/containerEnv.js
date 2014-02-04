

// this plugin inspects a containers environment vars 
// to get service information
function createService(container) {
    var env = createEnvironment(container);

    return {
        Port: 80,
        Environment: env.DNS_ENVIRONMENT || defaultEnvironment,
        TTL: env.DNS_TTL || defaultTTL,
        Service: env.DNS_SERVICE || cleanImageName(container.Image),
        Instance: env.DNS_INSTANCE || removeSlash(container.Name),
        Host: container.NetworkSettings.IpAddress
    }; 
}

// docker returns env vars in an array separated by =
// we need to convert this into a key value map
function createEnvironment(container) {
    var out = {};
    for (var i = 0; i < container.Config.Env.length; i++) {
        var full = container.Config.Env[i];
        var parts = full.split("=");

        if (parts[0].indexOf("DNS_") === 0) {
            out[parts[0]] = parts[1]; 
        }
    };
    return out;
}

#!/bin/bash

skydock -ttl 30 -docker "http://localhost:4243" -skydns "http://172.17.0.27:8080" -hostname dev

FROM debian:jessie

ADD skydock /usr/bin/skydock
ADD plugins/ /plugins
ENTRYPOINT ["skydock"]

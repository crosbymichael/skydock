FROM ubuntu

ADD skydock /usr/bin/skydock
ADD plugins/ /plugins
ENTRYPOINT ["skydock"]

FROM ubuntu

ADD skydock /usr/bin/skydock
ENTRYPOINT ["skydock"]

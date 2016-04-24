FROM crosbymichael/golang

# go get to download all the deps
RUN go get -u github.com/BradburyLab/skydock

ADD . /go/src/github.com/BradburyLab/skydock
ADD plugins/ /plugins

RUN cd /go/src/github.com/BradburyLab/skydock && go install . ./...

ENTRYPOINT ["/go/bin/skydock"]

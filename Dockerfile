FROM golang

ENV ROOT /app
WORKDIR $ROOT
ADD . $ROOT

RUN apt-get update
RUN apt-get upgrade -y

RUN go get -u cloud.google.com/go/pubsub google.golang.org/api/cloudiot/v1

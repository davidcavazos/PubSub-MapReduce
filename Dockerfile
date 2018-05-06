FROM golang

ARG credentials

WORKDIR /app
ADD . /app

ENV GOOGLE_APPLICATION_CREDENTIALS credentials.json

RUN apt-get update
RUN apt-get upgrade -y

RUN go get -u cloud.google.com/go/pubsub google.golang.org/api/cloudiot/v1

FROM golang:1.18.5 AS build

ENV GO111MODULE=on \
    GOOS=linux \
    GOARCH=amd64

RUN mkdir -p /src && \
    apk update && \
    apk add git

WORKDIR /src

COPY go.mod go.sum /src/
RUN go mod download

COPY . /src
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-http ./http

FROM debian:11.4-slim

RUN mkdir -p /app && \
    chgrp -R 0 /app && \
    chmod -R g=u /app

WORKDIR /app

COPY --from=build /src/proksi-http /app

CMD ["./proksi-http", "-main-upstream", "http://localhost:8080", "-test-upstream", "http://localhost:8081"]

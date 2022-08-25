FROM golang:1.18.5 AS build

ENV GO111MODULE=on \
    GOOS=linux \
    GOARCH=amd64

RUN mkdir -p /src

WORKDIR /src

COPY go.mod go.sum /src/
RUN go mod download

COPY . /src
RUN make build-http

FROM debian:11.4-slim

COPY --from=build /src/proksi-http /usr/local/bin/

CMD ["/usr/local/bin/proksi-http"]

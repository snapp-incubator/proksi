FROM golang:1.25.0 AS build

RUN mkdir -p /src

WORKDIR /src

COPY go.mod go.sum Makefile /src/
RUN make mod

COPY . /src
RUN make build-linux-http

FROM debian:11.4-slim

COPY --from=build /src/proksi-http /usr/local/bin/

CMD ["/usr/local/bin/proksi-http"]

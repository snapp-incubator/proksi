mod:
	go mod download

build-http:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-http ./http

build-linux-http:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-http ./http

test:
	go test ./...

lint:
	golangci-lint run ./...

check-suite: test lint

install-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

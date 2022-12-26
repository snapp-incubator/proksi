mod:
	go mod download

build-http:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-http ./http

build-linux-http:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-http ./http

build-redis:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-redis ./redis

build-linux-redis:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-redis ./redis

test:
	go test ./...

lint:
	golint ./...

check-suite: test

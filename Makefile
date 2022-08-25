build-http:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o proksi-http ./http

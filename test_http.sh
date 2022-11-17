#!/usr/bin/bash

pkill proksi

# shellcheck disable=SC2046
kill -9 $(lsof -t -i:8080)
# shellcheck disable=SC2046
kill -9 $(lsof -t -i:8081)
# shellcheck disable=SC2046
kill -9 $(lsof -t -i:9090)

set -e

# Run the Main and Test HTTP servers
go run ./server/servers.go &
sleep 3

# Run the Proksi-HTTP
go run http/http.go --config ./server/config.test.yaml &
sleep 1

for _ in {1..30}
do
    curl --location --request GET '127.0.0.1:9090/api/test' \
    --header 'Content-Type: application/json' \
    --data-raw '{
        "var1" : "test1",
        "var2" : 45556,
        "var3" : {
            "var4" : 12,
            "var5" : "test5"
        }
    }'
done


pkill proksi

# shellcheck disable=SC2046
kill -9 $(lsof -t -i:8080)
# shellcheck disable=SC2046
kill -9 $(lsof -t -i:8081)
# shellcheck disable=SC2046
kill -9 $(lsof -t -i:9090)

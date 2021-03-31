#!/bin/sh
go mod vendor
rm -f ./goosebin
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 exec go build ./cmd/goosebin/

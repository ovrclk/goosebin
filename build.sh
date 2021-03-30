#!/bin/sh
rm -f ./goosebin
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 exec go build ./cmd/goosebin/

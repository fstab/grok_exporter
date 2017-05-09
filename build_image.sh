#!/bin/bash

set +e

cd $GOPATH/src/github.com/fstab/grok_exporter
docker run --rm --net none -it -v $GOPATH/src/github.com/fstab/grok_exporter:/root/go/src/github.com/fstab/grok_exporter ubuntu:16.04 rm -rf /root/go/src/github.com/fstab/grok_exporter/dist
mkdir dist

export VERSION=0.2.2
export ARCH=linux-amd64
export VERSION_FLAGS="\
        -X github.com/fstab/grok_exporter/exporter.Version=$VERSION                          \
        -X github.com/fstab/grok_exporter/exporter.BuildDate=$(date +%Y-%m-%d)               \
        -X github.com/fstab/grok_exporter/exporter.Branch=$(git rev-parse --abbrev-ref HEAD) \
        -X github.com/fstab/grok_exporter/exporter.Revision=$(git rev-parse --short HEAD)    \
"

#--------------------------------------------------------------
# Make sure all tests run.
#--------------------------------------------------------------

go fmt $(go list ./... | grep -v /vendor/)
go test $(go list ./... | grep -v /vendor/)
cp -r docker/* dist
cp -a logstash-patterns-core/patterns dist
docker run -v $GOPATH/src/github.com/fstab/grok_exporter:/root/go/src/github.com/fstab/grok_exporter --net none --rm -ti fstab/grok_exporter-compiler compile-$ARCH.sh -ldflags "$VERSION_FLAGS" -o dist/grok_exporter

cd dist
docker build -t grok_exporter:$VERSION -t grok_exporter:latest .



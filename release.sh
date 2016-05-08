#!/bin/bash

set -e

#=======================================================================================
# This is supposed to run on OS X.
# The Darwin release is built natively, Linux and Windows are built in a Docker container
#========================================================================================

export VERSION=v0.0.1-SNAPSHOT

cd $GOPATH/src/github.com/fstab/grok_exporter
rm -rf dist

#--------------------------------------------------------------
# update the version file
#--------------------------------------------------------------

cat > version.go <<EOF
package main

const (
	VERSION = "$VERSION"
	BUILD_DATE = "`date +%Y-%m-%d`"
)
EOF
go fmt version.go > /dev/null

#--------------------------------------------------------------
# Make sure all tests run.
#--------------------------------------------------------------

go fmt ./...
go test ./...

#--------------------------------------------------------------
# Releases via Docker container (Windows, Linux)
#--------------------------------------------------------------

function make_release {
    ARCH=$1
    EXTENSION=$2
    echo "Building grok_exporter.$ARCH"
    mkdir -p dist/grok_exporter.$ARCH
    docker run -v $GOPATH:/root/go -t -i fstab/grok_exporter-compiler compile-$ARCH.sh -o dist/grok_exporter.$ARCH/grok_exporter$EXTENSION
    cp -a logstash-patterns-core/patterns dist/grok_exporter.$ARCH
    cp -a example dist/grok_exporter.$ARCH
    cd dist
    zip --quiet -r grok_exporter.$ARCH.zip grok_exporter.$ARCH
    rm -r grok_exporter.$ARCH
    cd ..
}

make_release windows-amd64 .exe
make_release linux-amd64

#--------------------------------------------------------------
# Native Darwin release
#--------------------------------------------------------------

ARCH=darwin-amd64

echo "Building grok_exporter.$ARCH"
mkdir -p dist/grok_exporter.$ARCH
go build -o dist/grok_exporter.$ARCH/grok_exporter .
cp -a logstash-patterns-core/patterns dist/grok_exporter.$ARCH
cp -a example dist/grok_exporter.$ARCH
cd dist
zip --quiet -r grok_exporter.$ARCH.zip grok_exporter.$ARCH
rm -r grok_exporter.$ARCH
cd ..

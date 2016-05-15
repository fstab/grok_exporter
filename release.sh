#!/bin/bash

set -e

#=======================================================================================
# This is supposed to run on OS X.
# The Darwin release is built natively, Linux and Windows are built in a Docker container
#========================================================================================

export VERSION=0.0.3-SNAPSHOT

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

go fmt $(go list ./... | grep -v /vendor/)
go test $(go list ./... | grep -v /vendor/)

function make_release {
    MACHINE=$1
    ARCH=$2
    EXTENSION=$3
    echo "Building grok_exporter-$VERSION.$ARCH"
    mkdir -p dist/grok_exporter-$VERSION.$ARCH
    if [ $MACHINE = "docker" ] ; then
        docker run -v $GOPATH/src/github.com/fstab/grok_exporter:/root/go/src/github.com/fstab/grok_exporter --net none --rm -ti fstab/grok_exporter-compiler compile-$ARCH.sh -o dist/grok_exporter-$VERSION.$ARCH/grok_exporter$EXTENSION
    else
        # export CGO_LDFLAGS=/usr/local/lib/libonig.a
        # TODO: For some reason CGO_LDFLAGS does not work on darwin. As a workaround, we set LDFLAGS directly in the header of regex.go.
        sed -i.bak 's;#cgo LDFLAGS: -L/usr/local/lib -lonig;#cgo LDFLAGS: /usr/local/lib/libonig.a;' vendor/github.com/moovweb/rubex/regex.go
        go build -o dist/grok_exporter-$VERSION.$ARCH/grok_exporter .
        mv vendor/github.com/moovweb/rubex/regex.go.bak vendor/github.com/moovweb/rubex/regex.go
    fi
    cp -a logstash-patterns-core/patterns dist/grok_exporter-$VERSION.$ARCH
    cp -a example dist/grok_exporter-$VERSION.$ARCH
    cd dist
    sed -i.bak s,/logstash-patterns-core/patterns,/patterns,g grok_exporter-$VERSION.$ARCH/example/*.yml
    rm grok_exporter-$VERSION.$ARCH/example/*.yml.bak
    zip --quiet -r grok_exporter-$VERSION.$ARCH.zip grok_exporter-$VERSION.$ARCH
    rm -r grok_exporter-$VERSION.$ARCH
    cd ..
}

make_release native darwin-amd64
make_release docker linux-amd64
make_release docker windows-amd64 .exe

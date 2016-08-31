#!/bin/bash

set -e

#=======================================================================================
# This is supposed to run on OS X.
# The Darwin release is built natively, Linux and Windows are built in a Docker container
#========================================================================================

cd $GOPATH/src/github.com/fstab/grok_exporter
rm -rf dist

export VERSION=0.1.2-SNAPSHOT

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

function make_release {
    MACHINE=$1
    ARCH=$2
    EXTENSION=$3
    echo "Building grok_exporter-$VERSION.$ARCH"
    mkdir -p dist/grok_exporter-$VERSION.$ARCH
    if [ $MACHINE = "docker" ] ; then
        docker run -v $GOPATH/src/github.com/fstab/grok_exporter:/root/go/src/github.com/fstab/grok_exporter --net none --rm -ti fstab/grok_exporter-compiler compile-$ARCH.sh -ldflags "$VERSION_FLAGS" -o dist/grok_exporter-$VERSION.$ARCH/grok_exporter$EXTENSION
    else
        # export CGO_LDFLAGS=/usr/local/lib/libonig.a
        # TODO: For some reason CGO_LDFLAGS does not work on darwin. As a workaround, we set LDFLAGS directly in the header of oniguruma.go.
        sed -i.bak 's;#cgo LDFLAGS: -L/usr/local/lib -lonig;#cgo LDFLAGS: /usr/local/lib/libonig.a;' exporter/oniguruma.go
        go build -ldflags "$VERSION_FLAGS" -o dist/grok_exporter-$VERSION.$ARCH/grok_exporter .
        mv exporter/oniguruma.go.bak exporter/oniguruma.go
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

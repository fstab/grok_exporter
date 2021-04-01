#!/bin/bash

set -e

GOVERSION=$(go version| awk ' { print $3; }'| sed 's/go\(.*\)\.\(.*\)/\1/')
if [ $(echo "$GOVERSION<1.11"|bc) -gt 0 ]; then 
    echo "grok_exporter uses Go 1.11 Modules. Please use Go version >= 1.11." >&2
    echo "Version found is $(go version)" >&2
    exit 1
fi

if git status | grep example/ ; then
    echo "error: untracked files in example directory" >&2
    exit 1
fi

# Needed for go1.11 and go1.12
export GO111MODULE=on

#=======================================================================================
# This is supposed to run on OS X.
# The Darwin release is built natively, Linux and Windows are built in a Docker container
#========================================================================================

SKIPTESTS="yes"                                     # Comment this var to skip tests
GOPATH=$HOME/go
SRCDIR=$(pwd)                                       # Source is current dir, in my case
#SRCDIR $GOPATH/src/github.com/fstab/grok_exporter  # you can change $SRCDIR at will...
#cd $SRCDIR

export VERSION=1.0.0-SNAPSHOT

DOCKER_IMAGENAME="fstab/grok_exporter-compiler-amd64"
#DOCKER_IMAGENAME="fstab/grok_exporter-compiler-amd64:v$VERSION"  docker image not found

export VERSION_FLAGS="\
        -X github.com/fstab/grok_exporter/exporter.Version=$VERSION
        -X github.com/fstab/grok_exporter/exporter.BuildDate=$(date +%Y-%m-%d)
        -X github.com/fstab/grok_exporter/exporter.Branch=$(git rev-parse --abbrev-ref HEAD)
        -X github.com/fstab/grok_exporter/exporter.Revision=$(git rev-parse --short HEAD)
"

#--------------------------------------------------------------
# Make sure all tests run.
#--------------------------------------------------------------

function run_tests {
    if [ -z ${SKIPTESTS+x} ]; then                   # if SKIPTESTS is undeclared skip tests
        go fmt ./... && go vet ./... && go test ./...
    fi
}

#--------------------------------------------------------------
# Helper functions
#--------------------------------------------------------------

function enable_legacy_static_linking {
    # The compile script in the Docker image sets CGO_LDFLAGS to libonig.a, which should make grok_exporter
    # statically linked with the Oniguruma library. However, this doesn't work on Darwin and CentOS 6.
    # As a workaround, we set LDFLAGS directly in the header of oniguruma.go.
    sed -i.bak 's;#cgo LDFLAGS: -L/usr/local/lib -lonig;#cgo LDFLAGS: /usr/local/lib/libonig.a;' oniguruma/oniguruma.go
}

function revert_legacy_static_linking {
    if [ -f oniguruma/oniguruma.go.bak ] ; then
        mv oniguruma/oniguruma.go.bak oniguruma/oniguruma.go
    fi
}

function cleanup {
    revert_legacy_static_linking
}

# Make sure revert_legacy_static_linking is called even if a compile error makes this script terminate early
trap cleanup EXIT

function create_zip_file {
    OUTPUT_DIR=$1
    cp -a logstash-patterns-core/patterns dist/$OUTPUT_DIR
    cp -a example dist/$OUTPUT_DIR
    cd dist
    sed -i.bak s,/logstash-patterns-core/patterns,/patterns,g $OUTPUT_DIR/example/*.yml
    rm $OUTPUT_DIR/example/*.yml.bak
    zip --quiet -r $OUTPUT_DIR.zip $OUTPUT_DIR
    rm -r $OUTPUT_DIR
    cd ..
}

function run_docker_linux_amd64 {
    docker run \
        -v $SRCDIR:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti "$DOCKER_IMAGENAME" \
        ./compile-linux.sh -ldflags "$VERSION_FLAGS" -o "dist/grok_exporter-$VERSION.linux-amd64/grok_exporter"
}

function run_docker_windows_amd64 {
    docker run \
        -v $SRCDIR:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti "$DOCKER_IMAGENAME" \
        ./compile-windows-amd64.sh -ldflags "$VERSION_FLAGS" -o "dist/grok_exporter-$VERSION.windows-amd64/grok_exporter.exe"
}

function run_docker_linux_arm64v8 {
    docker run \
        -v $SRCDIR:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti "fstab/grok_exporter-compiler-arm64v8:v$VERSION" \
        ./compile-linux.sh -ldflags "$VERSION_FLAGS" -o "dist/grok_exporter-$VERSION.linux-arm64v8/grok_exporter"
}

function run_docker_linux_arm32v6 {
    docker run \
        -v $SRCDIR:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti "fstab/grok_exporter-compiler-arm32v6:v$VERSION" \
        ./compile-linux.sh -ldflags "$VERSION_FLAGS" -o "dist/grok_exporter-$VERSION.linux-arm32v6/grok_exporter"
}

#--------------------------------------------------------------
# Release functions
#--------------------------------------------------------------

function release_linux_amd64 {
    echo "Building dist/grok_exporter-$VERSION.linux-amd64.zip"
    enable_legacy_static_linking
    run_docker_linux_amd64
    revert_legacy_static_linking
    create_zip_file grok_exporter-$VERSION.linux-amd64
}

function release_linux_arm64v8 {
    echo "Building dist/grok_exporter-$VERSION.linux-arm64v8.zip"
    run_docker_linux_arm64v8
    create_zip_file grok_exporter-$VERSION.linux-arm64v8
}

function release_linux_arm32v6 {
    echo "Building dist/grok_exporter-$VERSION.linux-arm32v6.zip"
    enable_legacy_static_linking
    run_docker_linux_arm32v6
    revert_legacy_static_linking
    create_zip_file grok_exporter-$VERSION.linux-arm32v6
}

function release_windows_amd64 {
    echo "Building dist/grok_exporter-$VERSION.windows-amd64.zip"
    run_docker_windows_amd64
    create_zip_file grok_exporter-$VERSION.windows-amd64
}

function release_darwin_amd64 {
    echo "Building dist/grok_exporter-$VERSION.darwin-amd64.zip"
    if [ "$(uname)" != "Darwin" ]; then
        echo "WARNING: Darwin releases can only be built on macOS." >&2
    else
        enable_legacy_static_linking
        go build -ldflags "$VERSION_FLAGS" -o dist/grok_exporter-$VERSION.darwin-amd64/grok_exporter .
        revert_legacy_static_linking
        create_zip_file grok_exporter-$VERSION.darwin-amd64
    fi
}

#--------------------------------------------------------------
# main
#--------------------------------------------------------------

case $1 in
    linux-amd64)
        rm -rf dist/grok_exporter-*.linux-amd64*
        run_tests
        release_linux_amd64
        ;;
    linux-arm64v8)
        rm -rf dist/grok_exporter-*.linux-arm64v8*
        run_tests
        release_linux_arm64v8
        ;;
    linux-arm32v6)
        rm -rf dist/grok_exporter-*.linux-arm32v6*
        run_tests
        release_linux_arm32v6
        ;;
    darwin-amd64)
        rm -rf dist/grok_exporter-*.darwin-amd64*
        run_tests
        release_darwin_amd64
        ;;
    windows-amd64)
        rm -rf dist/grok_exporter-*.windows-amd64*
        run_tests
        release_windows_amd64
        ;;
    all-amd64)
        rm -rf dist/grok_exporter-*.*-amd64*
        run_tests
        release_linux_amd64
        release_darwin_amd64
        release_windows_amd64
        ;;
    all)
        rm -rf dist/grok_exporter-*
        run_tests
        release_linux_amd64
        release_darwin_amd64
        release_windows_amd64
        release_linux_arm64v8
        release_linux_arm32v6
        ;;
    *)
        echo 'Usage: release.sh <arch>' >&2
        echo 'where <arch> can be:' >&2
        echo '    - linux-amd64' >&2
        echo '    - darwin-amd64' >&2
        echo '    - windows-amd64' >&2
        echo '    - linux-arm64v8' >&2
        echo '    - linux-arm32v6' >&2
        echo '    - all-amd64' >&2
        echo '    - all' >&2
        exit 255
esac

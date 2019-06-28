#!/bin/bash

set -e

if [[ $(go version) != *"go1.11"* && $(go version) != *"go1.12"* ]] ; then
    echo "grok_exporter uses Go 1.11 Modules. Please use Go version >= 1.11." >&2
    echo "Version found is $(go version)" >&2
    exit 1
fi

export GO111MODULE=on

#=======================================================================================
# This is supposed to run on OS X.
# The Darwin release is built natively, Linux and Windows are built in a Docker container
#========================================================================================

cd $GOPATH/src/github.com/fstab/grok_exporter

export VERSION=0.2.8

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
    go fmt ./... && go vet ./... && go test ./...
}

function create_vendor {
    go mod vendor
}

function remove_vendor {
    rm -fr ./vendor
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
    remove_vendor
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
        -v $GOPATH/src/github.com/fstab/grok_exporter:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti fstab/grok_exporter-compiler-amd64 \
        ./compile-linux.sh -ldflags "$VERSION_FLAGS" -o "dist/grok_exporter-$VERSION.linux-amd64/grok_exporter"
}

function run_docker_windows_amd64 {
    docker run \
        -v $GOPATH/src/github.com/fstab/grok_exporter:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti fstab/grok_exporter-compiler-amd64 \
        ./compile-windows-amd64.sh -ldflags "$VERSION_FLAGS" -o "dist/grok_exporter-$VERSION.windows-amd64/grok_exporter.exe"
}

function run_docker_linux_arm64v8 {
    docker run \
        -v $GOPATH/src/github.com/fstab/grok_exporter:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti fstab/grok_exporter-compiler-arm64v8 \
        ./compile-linux.sh -ldflags "$VERSION_FLAGS" -o "dist/grok_exporter-$VERSION.linux-arm64v8/grok_exporter"
}

function run_docker_linux_arm32v6 {
    docker run \
        -v $GOPATH/src/github.com/fstab/grok_exporter:/go/src/github.com/fstab/grok_exporter \
        --net none \
        --user $(id -u):$(id -g) \
        --rm -ti fstab/grok_exporter-compiler-arm32v6 \
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
        create_vendor
        release_linux_amd64
        remove_vendor
        ;;
    linux-arm64v8)
        rm -rf dist/grok_exporter-*.linux-arm64v8*
        run_tests
        create_vendor
        release_linux_arm64v8
        remove_vendor
        ;;
    linux-arm32v6)
        rm -rf dist/grok_exporter-*.linux-arm32v6*
        run_tests
        create_vendor
        release_linux_arm32v6
        remove_vendor
        ;;
    darwin-amd64)
        rm -rf dist/grok_exporter-*.darwin-amd64*
        run_tests
        create_vendor
        release_darwin_amd64
        remove_vendor
        ;;
    windows-amd64)
        rm -rf dist/grok_exporter-*.windows-amd64*
        run_tests
        create_vendor
        release_windows_amd64
        remove_vendor
        ;;
    all-amd64)
        rm -rf dist/grok_exporter-*.*-amd64*
        run_tests
        create_vendor
        release_linux_amd64
        release_darwin_amd64
        release_windows_amd64
        remove_vendor
        ;;
    all)
        rm -rf dist/grok_exporter-*
        run_tests
        create_vendor
        release_linux_amd64
        release_darwin_amd64
        release_windows_amd64
        release_linux_arm64v8
        release_linux_arm32v6
        remove_vendor
        ;;
    *)
        echo 'Usage: ./release.sh <arch>' >&2
        echo 'where <arch> can be:' >&2
        echo '    - linux-amd64' >&2
        echo '    - darwin-amd64' >&2
        echo '    - windows-amd64' >&2
        echo '    - linux-arm64v8' >&2
        echo '    - linux-arm32v6' >&2
        echo '    - all-amd64' >&2
        echo '    - all' >&2
        exit -1
esac

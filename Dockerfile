#############################
# Multi-Stage Build

FROM golang:stretch as builder

# Install system deps
#   We need this in order to build oniguruma.
#   The debian deb packages for onigurma do not install static libs
RUN apt-get update && apt-get -y install build-essential make autoconf libtool

# Oniguruma: fetch, build, and install static libs
RUN cd /tmp && \
    git clone https://github.com/kkos/oniguruma.git && \
    cd /tmp/oniguruma && \
    autoreconf -vfi && \
    ./configure && \
    make && \
    make install

# grok_exporter: fetch source code
RUN mkdir -p /go/src/github.com/fstab && \
    cd /go/src/github.com/fstab && \
    git clone https://github.com/fstab/grok_exporter.git

# Fetch Golang Dependencies
RUN cd /go/src/github.com/fstab/grok_exporter && \
  git submodule update --init --recursive && \
  go get


# Build Statically-Linked Binary
RUN cd /go/src/github.com/fstab/grok_exporter && \
  GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build \
    -ldflags "-w -extldflags \"-static\" \
    -X github.com/fstab/grok_exporter/exporter.Version=$VERSION \
    -X github.com/fstab/grok_exporter/exporter.BuildDate=$(date +%Y-%m-%d) \
    -X github.com/fstab/grok_exporter/exporter.Branch=$(git rev-parse --abbrev-ref HEAD) \
    -X github.com/fstab/grok_exporter/exporter.Revision=$(git rev-parse --short HEAD) \
    "

#############################
# Final-Stage Build

FROM alpine:latest

WORKDIR /app

COPY --from=builder /go/src/github.com/fstab/grok_exporter/grok_exporter \
     /app/grok_exporter
COPY --from=builder /go/src/github.com/fstab/grok_exporter/logstash-patterns-core \
     /app/logstash-patterns-core

EXPOSE 9144
ENTRYPOINT [ "/app/grok_exporter" ]

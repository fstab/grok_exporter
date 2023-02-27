FROM quay.io/centos/centos:8 as builder

RUN cd /etc/yum.repos.d/ && sed -i 's/mirrorlist/#mirrorlist/g' /etc/yum.repos.d/CentOS-* && sed -i 's|#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g' /etc/yum.repos.d/CentOS-*

RUN yum clean all && \
    yum update -y

#------------------------------------------------------------------------------
# Basic tools
#------------------------------------------------------------------------------

RUN yum install -y \
    curl \
    git \
    wget \
    vim \
    gcc \
    make

#------------------------------------------------------------------------------
# Create a statically linked Oniguruma library for Linux amd64
#------------------------------------------------------------------------------

# This will create /usr/local/lib/libonig.a

RUN cd /tmp && \
    curl -sLO https://github.com/kkos/oniguruma/releases/download/v6.9.5_rev1/onig-6.9.5-rev1.tar.gz && \
    tar xfz onig-6.9.5-rev1.tar.gz && \
    rm onig-6.9.5-rev1.tar.gz && \
    cd /tmp/onig-6.9.5 && \
    ./configure && \
    make && \
    make install && \
    cd / && \
    rm -r /tmp/onig-6.9.5

#------------------------------------------------------------------------------
# Go development
#------------------------------------------------------------------------------

# Install golang manually, so we get the latest version.

RUN cd /usr/local && \
    curl --fail -sLO https://dl.google.com/go/go1.17.2.linux-amd64.tar.gz && \
    tar xfz go1.17.2.linux-amd64.tar.gz && \
    rm go1.17.2.linux-amd64.tar.gz && \
    cd / && \
    mkdir -p go/bin go/pkg

ENV GOROOT="/usr/local/go" \
    GOPATH="/go" \
    GOCACHE=/tmp/.cache
ENV PATH="${GOROOT}/bin:${PATH}"
ENV PATH="${GOPATH}/bin:${PATH}"
ENV CGO_LDFLAGS=/usr/local/lib/libonig.a
WORKDIR /go/src/github.com/fstab/grok_exporter
COPY . .

RUN go mod download
RUN go build -o /bin/grok-exporter

FROM quay.io/sysdig/sysdig-mini-ubi:1.4.7 as ubi

COPY --from=builder /go/src/github.com/fstab/grok_exporter/logstash-patterns-core/patterns /patterns
COPY --from=builder /bin/grok-exporter /bin/grok-exporter
EXPOSE 9144

ENTRYPOINT [ "/bin/grok-exporter", "-config", "/grok/config.yml" ]
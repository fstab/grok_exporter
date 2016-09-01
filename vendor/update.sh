#!/bin/bash

set -e

# patches are created with
# diff -Naur proj_orig proj_patched

export VENDOR=$GOPATH/src/github.com/fstab/grok_exporter/vendor

cd $VENDOR
# remove all subdirectories
find . ! -path . -maxdepth 1 -type d | xargs rm -rf

###########################################################################
# github.com/prometheus/client_golang/prometheus
###########################################################################

cd $VENDOR
mkdir -p github.com/prometheus
cd github.com/prometheus
git clone https://github.com/prometheus/client_golang.git
cd client_golang
git checkout v0.8.0
rm -rf .git

# Dependency: github.com/prometheus/client_model/go

cd $VENDOR
mkdir -p github.com/prometheus
cd github.com/prometheus
git clone https://github.com/prometheus/client_model.git
cd client_model
git checkout fa8ad6fec33561be4280a8f0514318c79d7f6cb6
ls -A | grep -v go | xargs rm -rf

# Dependency: github.com/prometheus/procfs

cd $VENDOR
mkdir -p github.com/prometheus
cd github.com/prometheus
git clone https://github.com/prometheus/procfs.git
cd procfs
git checkout abf152e5f3e97f2fafac028d2cc06c1feb87ffa5
ls -A | grep -v '.go' | xargs rm -rf

# Dependency: github.com/prometheus/common/expfmt

cd $VENDOR
mkdir -p github.com/prometheus
cd github.com/prometheus
git clone https://github.com/prometheus/common.git
cd common
git checkout ebdfc6da46522d58825777cf1f90490a5b1ef1d8
ls -A | grep -v expfmt | grep -v internal | grep -v model | xargs rm -r
rm -r expfmt/testdata

# Dependency: github.com/matttproud/golang_protobuf_extensions/pbutil

cd $VENDOR
mkdir -p github.com/matttproud
cd github.com/matttproud
git clone https://github.com/matttproud/golang_protobuf_extensions.git
cd golang_protobuf_extensions
git checkout c12348ce28de40eed0136aa2b644d0ee0650e56c
ls -A | grep -v pbutil | xargs rm -r
rm pbutil/.gitignore pbutil/Makefile

# Dependency: github.com/beorn7/perks/quantile

cd $VENDOR
mkdir -p github.com/beorn7
cd github.com/beorn7
git clone https://github.com/beorn7/perks.git
cd perks
git checkout 4c0e84591b9aa9e6dcfdf3e020114cd81f89d5f9
rm -rf .git .gitignore histogram topk README.md

# Dependency: github.com/golang/protobuf/proto

cd $VENDOR
mkdir -p github.com/golang
cd github.com/golang
git clone https://github.com/golang/protobuf.git
cd protobuf
git checkout 7390af9dcd3c33042ebaf2474a1724a83cf1a7e6
rm -rf .git .gitignore Make* jsonpb protoc-gen-go ptypes

###########################################################################
# gopkg.in/yaml.v2
###########################################################################

cd $VENDOR
mkdir -p gopkg.in
cd gopkg.in
git clone https://gopkg.in/yaml.v2
cd yaml.v2
git checkout v2
rm -rf .git .travis.yml

###########################################################################
# golang.org/x/exp/winfsnotify
###########################################################################

cd $VENDOR
mkdir -p golang.org/x/
cd golang.org/x
git clone https://go.googlesource.com/exp
cd exp
git checkout 7be2ce36128ef1337a5348a7cb5a599830b42ac3
find . -type f | grep -v winfsnotify.go | xargs rm -f
find . -type d -empty -delete

###########################################################################

find $VENDOR -type f -name '*_test.go'  | xargs rm

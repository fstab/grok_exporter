#!/bin/bash

set -e

# patches are created with
# diff -Naur proj_orig proj_patched

export VENDOR=$GOPATH/src/github.com/fstab/grok_exporter/vendor

cd $VENDOR
# remove all subdirectories
find . ! -path . -maxdepth 1 -type d | xargs rm -rf

###########################################################################
# github.com/prometheus/client_golang
###########################################################################

cd $VENDOR
mkdir -p github.com/prometheus
cd github.com/prometheus
git clone https://github.com/prometheus/client_golang.git
cd client_golang
git checkout 28be15864ef9ba05d74fa6fd13b928fd250e8f01
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
git checkout bc0a4460d0fc2693fcdebafafbf07c6d18913b97
ls -A | grep -v expfmt | grep -v internal | grep -v model | xargs rm -rf

# Dependency: github.com/matttproud/golang_protobuf_extensions/pbutil

cd $VENDOR
mkdir -p github.com/matttproud
cd github.com/matttproud
git clone https://github.com/matttproud/golang_protobuf_extensions.git
cd golang_protobuf_extensions
git checkout v1.0.0
ls -A | grep -v pbutil | xargs rm -rf

# Dependency: github.com/beorn7/perks/quantile

cd $VENDOR
mkdir -p github.com/beorn7
cd github.com/beorn7
git clone https://github.com/beorn7/perks.git
cd perks
git checkout 3ac7bf7a47d159a033b107610db8a1b6575507a4
rm -rf .git .gitignore histogram topk README.md
patch -p1 < $VENDOR/perks.patch

# Dependency: github.com/golang/protobuf/proto

cd $VENDOR
mkdir -p github.com/golang
cd github.com/golang
git clone https://github.com/golang/protobuf.git
cd protobuf
git checkout c3cefd437628a0b7d31b34fe44b3a7a540e98527
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

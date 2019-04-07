How To Verify Releases
======================

Releases are built using the `./releases.sh` script, which relies on a couple of Docker images from [hub.docker.com/u/fstab/](https://hub.docker.com/u/fstab/). The Dockerfiles are maintained on [github.com/fstab/docker-grok_exporter-compiler](https://github.com/fstab/docker-grok_exporter-compiler).

In order to run the ARM builds on an AMD64 processor you need `binfmt-support` and `qemu-user-static`, which is enabled by default in Docker for Mac.

After building a release, I do a quick manual test to verify that the binaries work. This file contains some notes on how to manually verify the releases for the different platforms.

linux-arm32v6
-------------

On a Linux system, follow the instructions from [github.com/dhruvvyas90/qemu-rpi-kernel](https://github.com/dhruvvyas90/qemu-rpi-kernel) to set up an emulated Raspberry PI system:

```bash
#!/bin/bash

set -e

mkdir ~/qemu_vms
cd ~/qemu_vms
sudo apt-get install -y qemu-system-arm
curl -LO --fail http://downloads.raspberrypi.org/raspbian_lite/images/raspbian_lite-2017-12-01/2017-11-29-raspbian-stretch-lite.zip
unzip 2017-11-29-raspbian-stretch-lite.zip
curl -LO --fail https://github.com/dhruvvyas90/qemu-rpi-kernel/raw/master/versatile-pb.dtb
curl -LO --fail https://github.com/dhruvvyas90/qemu-rpi-kernel/raw/master/kernel-qemu-4.14.79-stretch

cat > run.sh <<EOF
qemu-system-arm \
    -kernel kernel-qemu-4.14.79-stretch \
    -cpu arm1176 \
    -m 256 \
    -M versatilepb \
    -dtb versatile-pb.dtb \
    -no-reboot \
    -append "root=/dev/sda2 panic=1 rootfstype=ext4 rw" \
    -net nic -net user,hostfwd=tcp::5022-:22 \
    -hda 2017-11-29-raspbian-stretch-lite.img
EOF
```

Some info on arm32v6, arm32v7, armhf, armel can be found on the [Raspbian FAQ](https://www.raspbian.org/RaspbianFAQ).

linux-arm64v8
-------------

Create a Server on [Scaleway](https://cloud.scaleway.com).

linux-amd64
-----------

Use a plain Ubuntu Docker image.

darwin-amd64
------------

This is my local development machine.

windows-amd64
-------------

Set up a VirtualBox VM.

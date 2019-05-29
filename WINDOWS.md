Windows
=======

The Windows release is created on Linux using cross compilation. The release is built with `./release.sh windows-amd64`, which runs the Go cross compiler in the `fstab/grok_exporter-compiler-amd64` Docker image.

Continuous integration for Windows is run on [AppVeyor](https://ci.appveyor.com/project/fstab/grok-exporter) as configured in `.appveyor.yml`.

However, if Windows tests fail it is sometimes useful to reproduce this locally in a Windows VM. The following describes how to set up a Windows 10 VM on VirtualBox for running Windows tests locally.

* Create a Windows 10 Home Edition VM using the official ISO from [https://www.microsoft.com/software-download/windows10](https://www.microsoft.com/software-download/windows10). You don't need a license key to do that. I used 8G RAM and 60G disk, but that might not be necessary.
* Install Go as an MSI from [https://golang.org/](https://golang.org/) and add `C:\Go\bin` to your `PATH`.
* Install `msys2-x86_64` from [https://www.msys2.org](https://www.msys2.org).
* Run the mingw64 shell in the msys2 terminal by double-clicking `C:\msys64\mingw64.exe`.
* The following steps must be performed in the msys2 terminal:
  * Repeatedly run `pacman -Syu` until all packages are up-to-date.
  * Run
    ```
    pacman -S base-devel \
              mingw-w64-i686-toolchain \
              mingw-w64-x86_64-toolchain \
              git \
              mingw-w64-i686-cmake \
              mingw-w64-x86_64-cmake
    ```
  * Download the latest `oniguruma` release from [https://github.com/kkos/oniguruma/releases](https://github.com/kkos/oniguruma/releases) and install it as follows (replace 6.9.2 with the current version):
    ```
    tar xfz onig-6.9.2.tar.gz
    cd onig-6.9.2
    ./configure --host=x86_64-w64-mingw32 --prefix=/C/msys64/mingw64
    make
    make install
    ```
  * Clone `grok_exporter` from [https://github.com/fstab/grok_exporter](https://github.com/fstab/grok_exporter). Build and run it as follows:
    ```
    git clone https://github.com/fstab/grok_exporter.git
    cd grok_exporter
    git submodule update --init --recursive
    alias go=/C/Go/bin/go
    go fmt ./...
    go vet ./...
    go test ./...
    go install
    ```
    This will create `grok_exporter.exe` in `C:\Users\...\go\bin\`.

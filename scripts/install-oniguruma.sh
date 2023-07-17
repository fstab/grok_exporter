curl -sLO https://github.com/kkos/oniguruma/releases/download/v6.9.5_rev1/onig-6.9.5-rev1.tar.gz
tar xfz onig-6.9.5-rev1.tar.gz
cd onig-6.9.5
./configure
make
make install

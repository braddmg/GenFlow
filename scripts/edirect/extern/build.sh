#!/bin/sh

# Public domain notice for all NCBI EDirect scripts is located at:
# https://www.ncbi.nlm.nih.gov/books/NBK179288/#chapter6.Public_Domain_Notice

# determine current platform
platform=""
osname=`uname -s`
cputype=`uname -m`
case "$osname-$cputype" in
  Linux-x86_64 )           platform=Linux ;;
  Darwin-x86_64 )          platform=Darwin ;;
  Darwin-*arm* )           platform=Silicon ;;
  CYGWIN_NT-* | MINGW*-* ) platform=CYGWIN_NT ;;
  Linux-*arm* )            platform=ARM ;;
  Linux-aarch64 )          platform=ARM64 ;;
  * )                      platform=UNSUPPORTED ;;
esac

vendor=false

while [ "$#" -ne 0 ]
do
  case "$1" in
    -vendor | vendor )
      vendor=true
      shift
      ;;
    * )
      break
      ;;
  esac
done

if [ ! -f "go.mod" ]
then
  go mod init extern
  # add explicit location to find local helper package
  echo "replace eutils => ../eutils" >> go.mod
  # build local eutils library
  go get eutils
fi
if [ ! -f "go.sum" ]
then
  go mod tidy
fi

# cache external dependencies
if [ "$vendor" = true ] && [ ! -d "vendor" ]
then
  go mod vendor -e
fi

# erase any existing executables in current directory
for plt in Darwin Silicon Linux CYGWIN_NT ARM ARM64
do
  rm -f *.$plt
done

# build all executables for current platform
for exc in *.go
do
  base=${exc%.go}
  go build -o "$base.$platform" "$base.go"
done

# will be using "go run", erase executables after test complication
for plt in Darwin Silicon Linux CYGWIN_NT ARM ARM64
do
  rm -f *.$plt
done

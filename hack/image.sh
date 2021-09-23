#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

VERSION=$(git describe --dirty --always --tags | sed 's/-/./g')
IMAGE=caelus:${VERSION}

docker build -t $IMAGE -f hack/Dockerfile .

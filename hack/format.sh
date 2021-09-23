#!/bin/bash

set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
GOFMT="gofmt -s -d -w"

find_files() {
  find . -not \( \
      \( \
        -wholename './_output' \
      \) -prune \
    \) -name '*.go'
}

find_files | xargs $GOFMT

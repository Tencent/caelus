#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

BASE_DIR=$(cd $(dirname $0)/.. && pwd)
source "${BASE_DIR}/hack/common.sh"
source "${BASE_DIR}/hack/lib/version.sh"

go build -o ${OUTPUT_PATH}/bin/${PACKAGE_NAME} \
  -ldflags "$(api::version::ldflags)" \
  ${PACKAGE}/cmd/${PACKAGE_NAME}

go build -o ${OUTPUT_PATH}/bin/${NM_OPERATOR_NAME} \
  -ldflags "$(api::version::ldflags)" \
  ${PACKAGE}/cmd/${NM_OPERATOR_NAME}

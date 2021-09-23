#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

PACKAGE_PREFIX="github.com/tencent"
PACKAGE_NAME="caelus"
PACKAGE="${PACKAGE_PREFIX}/${PACKAGE_NAME}"
OUTPUT_PATH="${BASE_DIR}/_output"
ADAPTER_NAME="caelus_metric_adapter"
NM_OPERATOR_NAME="nm-operator"
mkdir -p ${OUTPUT_PATH}
USER_ID=$(id -u)
GROUP_ID=$(id -g)

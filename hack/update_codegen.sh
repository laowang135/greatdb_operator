#!/usr/bin/env bash

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

MODULE=$1
GROUPVERSION=$2



SCRIPT_ROOT=$(pwd)

CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ./hack/code-generator)}
echo "${CODEGEN_PKG}"/generate-groups.sh "deepcopy,client,lister,informer" \
    ${MODULE}/pkg/client ${MODULE}/pkg/apis \
    ${GROUPVERSION} \
    --output-base="${SCRIPT_ROOT}" \
    --go-header-file="${SCRIPT_ROOT}"/hack/boilerplate.go.txt -v=5
    
bash "${CODEGEN_PKG}"/generate-groups.sh "deepcopy,client,lister,informer" \
    ${MODULE}/pkg/client ${MODULE}/pkg/apis \
    ${GROUPVERSION} \
    --output-base "${SCRIPT_ROOT}/../" \
    --go-header-file "${SCRIPT_ROOT}"/hack/boilerplate.go.txt -v=5

#####################样例 start##################################
# bash ./hack/update_codegen.sh  greatdb-operator greatdb:v1alpha1
#注意事项：
#MODULE需和go.mod文件内容一致
#"${CODEGEN_PKG}"/generate-groups.sh "deepcopy,client,informer,lister" \
#  sample-controller/pkg/generated sample-controller/pkg/apis \
#  samplecontroller:v1alpha1 \
#  --output-base "$(dirname "${BASH_SOURCE[0]}")/../.." \
#  --go-header-file "${SCRIPT_ROOT}"/hack/boilerplate.go.txt
#####################样例 end##################################
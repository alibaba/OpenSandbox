#!/bin/bash
# Copyright 2026 Alibaba Group Holding Ltd.
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

set -euxo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

KIND_CLUSTER="${KIND_CLUSTER:-opensandbox-e2e}"
KIND_K8S_VERSION="${KIND_K8S_VERSION:-v1.30.4}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-/tmp/opensandbox-kind-kubeconfig}"
E2E_NAMESPACE="${E2E_NAMESPACE:-opensandbox-e2e}"
SERVER_NAMESPACE="${SERVER_NAMESPACE:-opensandbox-system}"
PVC_NAME="${PVC_NAME:-opensandbox-e2e-pvc-test}"
PV_NAME="${PV_NAME:-opensandbox-e2e-pv-test}"
CONTROLLER_IMG="${CONTROLLER_IMG:-opensandbox/controller:e2e-local}"
SERVER_IMG="${SERVER_IMG:-opensandbox/server:e2e-local}"
EXECD_IMG="${EXECD_IMG:-opensandbox/execd:e2e-local}"
EGRESS_IMG="${EGRESS_IMG:-opensandbox/egress:e2e-local}"
CODE_INTERPRETER_IMG="${CODE_INTERPRETER_IMG:-opensandbox/code-interpreter:latest}"
SERVER_RELEASE="${SERVER_RELEASE:-opensandbox-server}"
SERVER_VALUES_FILE="${SERVER_VALUES_FILE:-/tmp/opensandbox-server-values.yaml}"
PORT_FORWARD_LOG="${PORT_FORWARD_LOG:-/tmp/opensandbox-server-port-forward.log}"

SERVER_IMG_REPOSITORY="${SERVER_IMG%:*}"
SERVER_IMG_TAG="${SERVER_IMG##*:}"

export KUBECONFIG="${KUBECONFIG_PATH}"
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "KUBECONFIG=${KUBECONFIG_PATH}" >> "${GITHUB_ENV}"
fi

cd "${REPO_ROOT}/kubernetes"
make setup-test-e2e KIND_CLUSTER="${KIND_CLUSTER}" KIND_K8S_VERSION="${KIND_K8S_VERSION}"
kind export kubeconfig --name "${KIND_CLUSTER}" --kubeconfig "${KUBECONFIG_PATH}"

# Build and load the latest controller code used by the Kubernetes runtime backend.
make docker-build-controller CONTROLLER_IMG="${CONTROLLER_IMG}"
kind load docker-image --name "${KIND_CLUSTER}" "${CONTROLLER_IMG}"
make install
make deploy CONTROLLER_IMG="${CONTROLLER_IMG}"
kubectl wait --for=condition=available --timeout=180s deployment/opensandbox-controller-manager -n opensandbox-system
cd "${REPO_ROOT}"

# Build sandbox-side control plane images from the current workspace so E2E exercises latest server/runtime code.
docker build -f server/Dockerfile -t "${SERVER_IMG}" server
docker build -f components/execd/Dockerfile -t "${EXECD_IMG}" "${REPO_ROOT}"
docker build -f components/egress/Dockerfile -t "${EGRESS_IMG}" "${REPO_ROOT}"
docker pull "${CODE_INTERPRETER_IMG}"

kind load docker-image --name "${KIND_CLUSTER}" "${SERVER_IMG}"
kind load docker-image --name "${KIND_CLUSTER}" "${EXECD_IMG}"
kind load docker-image --name "${KIND_CLUSTER}" "${EGRESS_IMG}"
kind load docker-image --name "${KIND_CLUSTER}" "${CODE_INTERPRETER_IMG}"

kubectl get namespace "${E2E_NAMESPACE}" >/dev/null 2>&1 || kubectl create namespace "${E2E_NAMESPACE}"

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ${PV_NAME}
spec:
  capacity:
    storage: 2Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: manual
  hostPath:
    path: /tmp/${PV_NAME}
    type: DirectoryOrCreate
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${PVC_NAME}
  namespace: ${E2E_NAMESPACE}
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: manual
  resources:
    requests:
      storage: 1Gi
  volumeName: ${PV_NAME}
EOF

kubectl wait --for=jsonpath='{.status.phase}'=Bound --timeout=120s "pvc/${PVC_NAME}" -n "${E2E_NAMESPACE}"

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: opensandbox-e2e-pvc-seed
  namespace: ${E2E_NAMESPACE}
spec:
  restartPolicy: Never
  containers:
    - name: seed
      image: alpine:3.20
      command:
        - /bin/sh
        - -c
        - |
          set -eux
          mkdir -p /data/datasets/train
          echo 'pvc-marker-data' > /data/marker.txt
          echo 'pvc-subpath-marker' > /data/datasets/train/marker.txt
      volumeMounts:
        - name: pvc
          mountPath: /data
  volumes:
    - name: pvc
      persistentVolumeClaim:
        claimName: ${PVC_NAME}
EOF

kubectl wait --for=jsonpath='{.status.phase}'=Succeeded --timeout=120s pod/opensandbox-e2e-pvc-seed -n "${E2E_NAMESPACE}"
kubectl delete pod/opensandbox-e2e-pvc-seed -n "${E2E_NAMESPACE}" --ignore-not-found=true

cat <<EOF > "${SERVER_VALUES_FILE}"
server:
  image:
    repository: ${SERVER_IMG_REPOSITORY}
    tag: "${SERVER_IMG_TAG}"
    pullPolicy: IfNotPresent
  replicaCount: 1
  resources:
    limits:
      cpu: "1"
      memory: 2Gi
    requests:
      cpu: "250m"
      memory: 512Mi
configToml: |
  [server]
  host = "0.0.0.0"
  port = 80
  log_level = "INFO"
  api_key = ""

  [runtime]
  type = "kubernetes"
  execd_image = "${EXECD_IMG}"

  [egress]
  image = "${EGRESS_IMG}"

  [kubernetes]
  namespace = "${E2E_NAMESPACE}"
  workload_provider = "batchsandbox"
  sandbox_create_timeout_seconds = 180
  sandbox_create_poll_interval_seconds = 1.0
  batchsandbox_template_file = "/etc/opensandbox/example.batchsandbox-template.yaml"

  [storage]
  allowed_host_paths = []
EOF

kubectl get namespace "${SERVER_NAMESPACE}" >/dev/null 2>&1 || kubectl create namespace "${SERVER_NAMESPACE}"
python3 - <<'PY' "${REPO_ROOT}" "${SERVER_VALUES_FILE}"
import subprocess
import sys

try:
    import tomllib
except ModuleNotFoundError:
    import tomli as tomllib

repo_root, values_file = sys.argv[1], sys.argv[2]
chart_path = f"{repo_root}/kubernetes/charts/opensandbox-server"

rendered = subprocess.run(
    ["helm", "template", "opensandbox-server", chart_path, "-f", values_file],
    check=True,
    capture_output=True,
    text=True,
).stdout

config_lines = []
capturing = False
for line in rendered.splitlines():
    if line == "  config.toml: |":
        capturing = True
        continue
    if capturing:
        if line.startswith("---"):
            break
        if line.startswith("    "):
            config_lines.append(line[4:])
            continue
        if line.strip() == "":
            config_lines.append("")
            continue
        break

if not config_lines:
    raise RuntimeError("Failed to extract config.toml from rendered Helm manifest")

tomllib.loads("\n".join(config_lines) + "\n")
PY

helm upgrade --install "${SERVER_RELEASE}" "${REPO_ROOT}/kubernetes/charts/opensandbox-server" \
  --namespace "${SERVER_NAMESPACE}" \
  --create-namespace \
  -f "${SERVER_VALUES_FILE}"
if ! kubectl wait --for=condition=available --timeout=180s deployment/opensandbox-server -n "${SERVER_NAMESPACE}"; then
  kubectl get pods -n "${SERVER_NAMESPACE}" -o wide || true
  kubectl describe deployment/opensandbox-server -n "${SERVER_NAMESPACE}" || true
  kubectl describe pods -n "${SERVER_NAMESPACE}" -l app.kubernetes.io/name=opensandbox-server || true
  kubectl logs -n "${SERVER_NAMESPACE}" deployment/opensandbox-server --all-containers=true || true
  exit 1
fi

kubectl port-forward -n "${SERVER_NAMESPACE}" svc/opensandbox-server 8080:80 >"${PORT_FORWARD_LOG}" 2>&1 &
PORT_FORWARD_PID=$!
trap 'kill "${PORT_FORWARD_PID}" >/dev/null 2>&1 || true' EXIT

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/health >/dev/null; then
    break
  fi
  sleep 2
done
curl -fsS http://127.0.0.1:8080/health >/dev/null

# Build local lifecycle client code before running the Python E2E suite.
cd sdks/sandbox/python
make generate-api
cd ../../..

export OPENSANDBOX_TEST_DOMAIN="localhost:8080"
export OPENSANDBOX_TEST_PROTOCOL="http"
export OPENSANDBOX_TEST_API_KEY=""
export OPENSANDBOX_SANDBOX_DEFAULT_IMAGE="${CODE_INTERPRETER_IMG}"
export OPENSANDBOX_E2E_RUNTIME="kubernetes"
export OPENSANDBOX_TEST_USE_SERVER_PROXY="true"
export OPENSANDBOX_TEST_PVC_NAME="${PVC_NAME}"

cd tests/python
uv sync --all-extras --refresh
make test

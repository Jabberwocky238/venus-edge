#!/bin/bash
set -euo pipefail

# bash <(curl -sSL https://raw.githubusercontent.com/jabberwocky238/venus-edge/main/deploy/prepare.sh)
TARGET_DIR="${PWD}"
MODE_DIR="${HOME}/.venus-edge"
MODE_FILE="${MODE_DIR}/mode"
BASE_URL="${LUNA_EDGE_DEPLOY_BASE_URL:-https://raw.githubusercontent.com/jabberwocky238/venus-edge/main/deploy}"

FILES=(
  "ns.yaml"
  "venus-edge-master.yaml"
  "venus-edge-agent.yaml"
  "venus-edge-agent-cilium-clustermesh.yaml"
  "run.sh"
)

mkdir -p "${MODE_DIR}"
if [[ ! -f "${MODE_FILE}" ]]; then
  printf 'default\n' > "${MODE_FILE}"
fi

for file in "${FILES[@]}"; do
  echo "downloading ${file}"
  curl -fsSL "${BASE_URL}/${file}" -o "${TARGET_DIR}/${file}"
done

echo "download dir: ${TARGET_DIR}"
echo "mode file: ${MODE_FILE}"
echo "current mode: $(cat "${MODE_FILE}")"

chmod +x "${TARGET_DIR}/run.sh"
bash "${TARGET_DIR}/run.sh" up ns
echo "prepare complete"

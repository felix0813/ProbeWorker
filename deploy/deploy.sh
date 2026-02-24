#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "ENV file not found: $ENV_FILE"
  echo "Copy .example-env to .env and fill required variables."
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

required_vars=(
  DEPLOY_USER
  DEPLOY_GROUP
  INSTALL_DIR
  WEB_BASE_PATH
  API_PORT
  PROBE_CONFIG_FILE
)
for var in "${required_vars[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "Missing required env var: $var"
    exit 1
  fi
done

SERVICE_DIR="${SERVICE_DIR:-/etc/systemd/system}"
NGINX_CONF_DIR="${NGINX_CONF_DIR:-/etc/nginx/conf.d}"
NGINX_CONF_NAME="${NGINX_CONF_NAME:-probeworker.conf}"
API_SERVICE_NAME="${API_SERVICE_NAME:-probeworker-api}"
PROBE_SERVICE_NAME="${PROBE_SERVICE_NAME:-probeworker-worker}"
API_LISTEN_HOST="${API_LISTEN_HOST:-127.0.0.1}"
PROBE_BIN_NAME="${PROBE_BIN_NAME:-probe-worker}"
API_BIN_NAME="${API_BIN_NAME:-probe-api}"
WEB_BASE_PATH="${WEB_BASE_PATH%/}"
if [[ -z "$WEB_BASE_PATH" ]]; then WEB_BASE_PATH="/probe"; fi

BIN_DIR="$INSTALL_DIR/bin"
WEB_DIR="$INSTALL_DIR/web"
DATA_DIR="$INSTALL_DIR/data"
RUNTIME_ENV_FILE="$INSTALL_DIR/runtime.env"

sudo install -d -m 755 -o "$DEPLOY_USER" -g "$DEPLOY_GROUP" "$BIN_DIR" "$WEB_DIR" "$DATA_DIR"

(
  cd "$ROOT_DIR/api"
  go build -o "$BIN_DIR/$API_BIN_NAME" .
)
(
  cd "$ROOT_DIR/Probe"
  go build -o "$BIN_DIR/$PROBE_BIN_NAME" .
)

rsync -a --delete "$ROOT_DIR/web/" "$WEB_DIR/"
sudo chown -R "$DEPLOY_USER:$DEPLOY_GROUP" "$INSTALL_DIR"

cat | sudo tee "$RUNTIME_ENV_FILE" >/dev/null <<RUNTIME
API_LISTEN_ADDR=${API_LISTEN_HOST}:${API_PORT}
API_WEB_DIR=${WEB_DIR}
API_DATA_FILE=${DATA_DIR}/probes.json
RUNTIME

cat | sudo tee "$SERVICE_DIR/${API_SERVICE_NAME}.service" >/dev/null <<UNIT
[Unit]
Description=ProbeWorker API service
After=network.target

[Service]
Type=simple
User=${DEPLOY_USER}
Group=${DEPLOY_GROUP}
EnvironmentFile=${RUNTIME_ENV_FILE}
ExecStart=${BIN_DIR}/${API_BIN_NAME}
Restart=always
RestartSec=3
WorkingDirectory=${INSTALL_DIR}

[Install]
WantedBy=multi-user.target
UNIT

cat | sudo tee "$SERVICE_DIR/${PROBE_SERVICE_NAME}.service" >/dev/null <<UNIT
[Unit]
Description=ProbeWorker probe worker service
After=network.target

[Service]
Type=simple
User=${DEPLOY_USER}
Group=${DEPLOY_GROUP}
ExecStart=${BIN_DIR}/${PROBE_BIN_NAME} ${PROBE_CONFIG_FILE}
Restart=always
RestartSec=3
WorkingDirectory=${INSTALL_DIR}

[Install]
WantedBy=multi-user.target
UNIT

cat | sudo tee "$NGINX_CONF_DIR/$NGINX_CONF_NAME" >/dev/null <<NGINX
location = ${WEB_BASE_PATH} {
    return 301 ${WEB_BASE_PATH}/;
}

location ${WEB_BASE_PATH}/ {
    proxy_pass http://${API_LISTEN_HOST}:${API_PORT}/;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
}
NGINX

sudo systemctl daemon-reload
sudo systemctl enable --now "${API_SERVICE_NAME}.service"
sudo systemctl enable --now "${PROBE_SERVICE_NAME}.service"
sudo nginx -t
sudo systemctl reload nginx

echo "Deploy done."
echo "API service: ${API_SERVICE_NAME}.service"
echo "Worker service: ${PROBE_SERVICE_NAME}.service"
echo "Nginx path: ${WEB_BASE_PATH}/"

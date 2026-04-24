#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/Spittingjiu/sui-go.git"
INSTALL_DIR="/opt/sui-go"
BIN_PATH="/usr/local/bin/sui-go"
SERVICE_PATH="/etc/systemd/system/sui-go.service"
ENV_PATH="/etc/default/sui-go"

need_root() {
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    echo "请用 root 运行: sudo bash install.sh"
    exit 1
  fi
}

ensure_deps() {
  if ! command -v git >/dev/null 2>&1; then
    apt-get update -y && apt-get install -y git
  fi
  if ! command -v go >/dev/null 2>&1; then
    apt-get update -y && apt-get install -y golang-go
  fi
}

install_code() {
  mkdir -p "$INSTALL_DIR"
  if [[ -d "$INSTALL_DIR/.git" ]]; then
    git -C "$INSTALL_DIR" pull --ff-only
  else
    rm -rf "$INSTALL_DIR"
    git clone "$REPO" "$INSTALL_DIR"
  fi
}

build_bin() {
  cd "$INSTALL_DIR"
  go build -o sui-go ./cmd/sui-go
  install -m 0755 sui-go "$BIN_PATH"
}

write_env() {
  if [[ ! -f "$ENV_PATH" ]]; then
    cat > "$ENV_PATH" <<'EOF'
ADDR=:8811
DB_FILE=/opt/sui-go/data/sui-go.db
PANEL_USER=admin
PANEL_PASS=admin123
EOF
  fi
}

write_service() {
  cat > "$SERVICE_PATH" <<'EOF'
[Unit]
Description=SUI-Go Panel Service
After=network.target

[Service]
Type=simple
EnvironmentFile=-/etc/default/sui-go
WorkingDirectory=/opt/sui-go
ExecStart=/usr/local/bin/sui-go
Restart=always
RestartSec=2
User=root

[Install]
WantedBy=multi-user.target
EOF
}

main() {
  need_root
  ensure_deps
  install_code
  build_bin
  mkdir -p /opt/sui-go/data
  write_env
  write_service

  systemctl daemon-reload
  systemctl enable --now sui-go
  systemctl status sui-go --no-pager -n 30 || true

  echo
  echo "安装完成。"
  echo "服务: systemctl status sui-go"
  echo "环境配置: $ENV_PATH"
  echo "默认地址: http://<IP>:8811"
}

main "$@"

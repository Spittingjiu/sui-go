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
ADDR=:18811
DB_FILE=/opt/sui-go/data/sui-go.db
PANEL_USER=admin
PANEL_PASS=admin123
XRAY_CONFIG_OUT=/usr/local/x-ui/bin/config.json
# 示例：XRAY_RELOAD_CMD=systemctl restart x-ui
XRAY_RELOAD_CMD=
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

write_sui_cli() {
cat > /usr/local/bin/sui <<'SUI_GO_MENU_EOF'
#!/usr/bin/env bash
set -euo pipefail

ENV_FILE=/etc/default/sui-go
SERVICE=sui-go.service
INSTALL_DIR=/opt/sui-go
BIN_PATH=/usr/local/bin/sui-go
DB_FILE_DEFAULT=/opt/sui-go/data/sui-go.db
XRAY_SERVICE=xray

need_root(){
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    echo "请用 root 运行：sudo sui"
    exit 1
  fi
}

pause(){ read -r -p "回车继续" _ || true; }

get_env(){
  local k="$1" d="${2:-}"
  local v=""
  if [[ -f "$ENV_FILE" ]]; then
    v=$(grep -E "^${k}=" "$ENV_FILE" 2>/dev/null | tail -n1 | cut -d= -f2- || true)
  fi
  printf '%s' "${v:-$d}"
}

set_kv(){
  local k="$1" v="$2"
  touch "$ENV_FILE"
  if grep -q "^${k}=" "$ENV_FILE" 2>/dev/null; then
    sed -i "s#^${k}=.*#${k}=${v}#" "$ENV_FILE"
  else
    echo "${k}=${v}" >> "$ENV_FILE"
  fi
}

reload_apply(){
  systemctl daemon-reload
  systemctl restart "$SERVICE"
}

panel_addr(){
  local addr port
  addr=$(get_env ADDR ':18811')
  port="${addr##*:}"
  [[ "$port" =~ ^[0-9]+$ ]] || port=18811
  echo "http://127.0.0.1:${port}"
}

api_login_token(){
  local base user pass
  base=$(panel_addr)
  user="${1:-$(get_env PANEL_USER admin)}"
  pass="${2:-$(get_env PANEL_PASS admin123)}"
  curl -fsS -X POST "$base/auth/login" -H 'content-type: application/json' \
    --data "$(jq -nc --arg username "$user" --arg password "$pass" '{username:$username,password:$password}')" \
    | jq -r '.token // empty'
}

current_user_from_db(){
  local db
  db=$(get_env DB_FILE "$DB_FILE_DEFAULT")
  if command -v sqlite3 >/dev/null 2>&1 && [[ -f "$db" ]]; then
    sqlite3 "$db" 'select username from user_dbs order by id limit 1;' 2>/dev/null | head -n1
  fi
}

current_panel_info(){
  local db user port addr
  db=$(get_env DB_FILE "$DB_FILE_DEFAULT")
  user=$(current_user_from_db)
  user=${user:-$(get_env PANEL_USER admin)}
  addr=$(get_env ADDR ':18811')
  port="${addr##*:}"; [[ "$port" =~ ^[0-9]+$ ]] || port=18811
  echo "当前用户名: $user"
  echo "当前端口: $port"
  echo "面板地址: http://<服务器IP>:$port"
  echo "本机地址: http://127.0.0.1:$port"
  echo "服务状态: $(systemctl is-active "$SERVICE" 2>/dev/null || echo unknown) / $(systemctl is-enabled "$SERVICE" 2>/dev/null || echo disabled)"
  if command -v xray >/dev/null 2>&1; then
    echo "Xray版本: $(xray version 2>/dev/null | head -n1 || true)"
  fi
  if [[ -f "$db" ]]; then
    echo "数据库: $db"
  fi
}

change_account(){
  local old_user old_pass new_user new_pass token base
  base=$(panel_addr)
  old_user=$(current_user_from_db); old_user=${old_user:-$(get_env PANEL_USER admin)}
  read -r -p "当前用户名（默认 $old_user）: " old_user_in
  old_user="${old_user_in:-$old_user}"
  read -r -s -p "当前密码: " old_pass; echo
  [[ -n "$old_pass" ]] || { echo "当前密码不能为空"; return 1; }
  token=$(api_login_token "$old_user" "$old_pass" || true)
  [[ -n "$token" ]] || { echo "登录验证失败，未修改"; return 1; }
  read -r -p "新用户名（留空不改）: " new_user
  if [[ -n "${new_user:-}" ]]; then
    curl -fsS -X POST "$base/api/panel/change-username" -H "Authorization: Bearer $token" -H 'content-type: application/json' \
      --data "$(jq -nc --arg username "$new_user" '{username:$username}')" >/dev/null
    set_kv PANEL_USER "$new_user"
    old_user="$new_user"
    echo "用户名已更新"
  fi
  read -r -s -p "新密码（留空不改）: " new_pass; echo
  if [[ -n "${new_pass:-}" ]]; then
    curl -fsS -X POST "$base/api/panel/change-password" -H "Authorization: Bearer $token" -H 'content-type: application/json' \
      --data "$(jq -nc --arg oldPassword "$old_pass" --arg newPassword "$new_pass" '{oldPassword:$oldPassword,newPassword:$newPassword}')" >/dev/null
    set_kv PANEL_PASS "$new_pass"
    echo "密码已更新"
  fi
}

change_port(){
  local cur port
  cur=$(get_env ADDR ':18811'); cur="${cur##*:}"
  read -r -p "新端口（当前 $cur）: " port
  [[ "$port" =~ ^[0-9]+$ ]] || { echo "端口必须是数字"; return 1; }
  if (( port < 1 || port > 65535 )); then echo "端口范围应为 1-65535"; return 1; fi
  set_kv ADDR ":$port"
  reload_apply
  echo "已更新端口为 $port"
}

opt_bbr(){
  cat >/etc/sysctl.d/99-sui-go-bbr.conf <<'EOT'
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
EOT
  modprobe tcp_bbr || true
  sysctl --system >/dev/null
  echo "已启用 BBR + fq"
}

connect_sub(){
  local sub_url sub_user sub_pass source_name token base
  base=$(panel_addr)
  token=$(api_login_token || true)
  if [[ -z "$token" ]]; then
    echo "无法自动登录面板，请先确认 /etc/default/sui-go 里的 PANEL_USER/PANEL_PASS 是否正确。"
    return 1
  fi
  read -r -p "请输入 sui-sub 地址（如: https://sub.example.com）: " sub_url
  sub_url="${sub_url%/}"
  [[ -n "$sub_url" ]] || { echo "sui-sub 地址不能为空"; return 1; }
  read -r -p "请输入 sui-sub 用户名: " sub_user
  [[ -n "$sub_user" ]] || { echo "用户名不能为空"; return 1; }
  read -r -s -p "请输入 sui-sub 密码: " sub_pass; echo
  [[ -n "$sub_pass" ]] || { echo "密码不能为空"; return 1; }
  read -r -p "写入到 sub 的源名称（默认: sui-go）: " source_name
  source_name="${source_name:-sui-go}"
  curl -fsS -X POST "$base/api/panel/connect-sub" -H "Authorization: Bearer $token" -H 'content-type: application/json' \
    --data "$(jq -nc --arg subUrl "$sub_url" --arg subUsername "$sub_user" --arg subPassword "$sub_pass" --arg sourceName "$source_name" '{subUrl:$subUrl,subUsername:$subUsername,subPassword:$subPassword,sourceName:$sourceName}')" \
    | jq -r '.msg // .obj // .'
}

update_panel(){
  if [[ ! -d "$INSTALL_DIR/.git" ]]; then
    echo "$INSTALL_DIR 不是 git 仓库，无法自动更新面板源码。"
    return 1
  fi
  cd "$INSTALL_DIR"
  git fetch --all --prune
  git pull --ff-only origin "$(git rev-parse --abbrev-ref HEAD || echo main)"
  go build -o sui-go ./cmd/sui-go
  install -m 0755 sui-go "$BIN_PATH"
  reload_apply
  echo "sui-go 面板已更新并重启"
}

xray_update_menu(){
  local base token info cur stable dev ch ver
  base=$(panel_addr)
  token=$(api_login_token || true)
  [[ -n "$token" ]] || { echo "无法自动登录面板"; return 1; }
  info=$(curl -fsS "$base/api/system/xray/version-current" -H "Authorization: Bearer $token")
  cur=$(echo "$info" | jq -r '.obj.current // "unknown"')
  stable=$(echo "$info" | jq -r '.obj.stableLatest // empty')
  dev=$(echo "$info" | jq -r '.obj.devLatest // empty')
  echo "当前版本: $cur"
  echo "稳定版最新: ${stable:-未知}"
  echo "开发版最新: ${dev:-未知}"
  echo "1) 更新稳定版 (${stable:-未知})"
  echo "2) 更新开发版 (${dev:-未知})"
  echo "0) 返回"
  read -r -p "选择: " ch
  case "$ch" in
    1) ver="$stable" ;;
    2) ver="$dev" ;;
    0) return 0 ;;
    *) echo "无效选择"; return 1 ;;
  esac
  [[ -n "$ver" && "$ver" != "未知" ]] || { echo "目标版本为空"; return 1; }
  read -r -p "确认更新 Xray 到 $ver ? 输入 YES: " ok
  [[ "$ok" == "YES" ]] || { echo "已取消"; return 0; }
  curl -fsS -X POST "$base/api/system/xray/switch" -H "Authorization: Bearer $token" -H 'content-type: application/json' \
    --data "$(jq -nc --arg version "$ver" '{version:$version}')" | jq .
}

uninstall_sui_go(){
  local keep_data="Y"
  echo "警告：此操作将卸载 sui-go 面板与 systemd 服务。"
  read -r -p "是否保留数据目录 /opt/sui-go/data？[Y/n]: " keep_data
  keep_data="${keep_data:-Y}"
  read -r -p "确认卸载请输入 YES: " confirm
  [[ "$confirm" == "YES" ]] || { echo "已取消卸载"; return 0; }
  systemctl stop "$SERVICE" >/dev/null 2>&1 || true
  systemctl disable "$SERVICE" >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/sui-go.service
  systemctl daemon-reload
  rm -f /usr/local/bin/sui-go
  rm -f /usr/local/bin/sui /usr/local/sbin/sui /usr/bin/sui
  rm -f /etc/profile.d/sui-go.sh /etc/profile.d/sui.sh
  rm -f /etc/default/sui-go
  if [[ "$keep_data" =~ ^[Nn]$ ]]; then
    rm -rf /opt/sui-go
    echo "已删除程序与数据目录"
  else
    find /opt/sui-go -mindepth 1 -maxdepth 1 ! -name data -exec rm -rf {} + 2>/dev/null || true
    echo "已卸载程序，保留 /opt/sui-go/data"
  fi
  echo "sui-go 卸载完成。若当前 shell 仍能执行 sui，请执行：hash -r。"
  exit 0
}

main_menu(){
  need_root
  while true; do
    echo "===== SUI-Go 菜单 ====="
    echo "1) 修改面板账号密码"
    echo "2) 显示当前用户信息"
    echo "3) 修改面板端口"
    echo "4) 启用 BBR + fq"
    echo "5) 一键对接 sub"
    echo "6) 更新 sui-go 面板"
    echo "7) Xray 更新（稳定版/开发版）"
    echo "8) 一键卸载 sui-go"
    echo "0) 退出"
    read -r -p "选择: " c
    case "$c" in
      1) change_account; pause ;;
      2) current_panel_info; pause ;;
      3) change_port; pause ;;
      4) opt_bbr; pause ;;
      5) connect_sub; pause ;;
      6) update_panel; pause ;;
      7) xray_update_menu; pause ;;
      8) uninstall_sui_go ;;
      0) exit 0 ;;
      *) echo "无效选择"; pause ;;
    esac
  done
}

main_menu "$@"

SUI_GO_MENU_EOF
chmod +x /usr/local/bin/sui
}

main() {
  need_root
  ensure_deps
  install_code
  build_bin
  mkdir -p /opt/sui-go/data
  write_env
  write_service
  write_sui_cli

  systemctl daemon-reload
  systemctl enable --now sui-go
  systemctl status sui-go --no-pager -n 30 || true

  echo
  echo "安装完成。"
  echo "服务: systemctl status sui-go"
  echo "环境配置: $ENV_PATH"
  echo "默认地址: http://<IP>:18811"
}

main "$@"

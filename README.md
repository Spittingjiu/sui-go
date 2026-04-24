# sui-go

Go 版本的 SUI（对标 s-ui 的 Go 架构方向），用于逐步替换现有 Node 面板。

## 当前状态

当前为 **Phase-3 迭代版**：
- Go 原生 HTTP 服务
- SQLite 持久化（gorm）
- 认证接口 `POST /auth/login`（Bearer Token 访问受保护 API）
- inbounds 接口：list/add/get/update/delete
- 协议支持（当前）：hysteria(v2)/vless/vmess/trojan/shadowsocks
- vless 细项（当前）：`network=xhttp`、`security=reality` 基础字段
- xray 配置导出接口：`GET /api/xray/config`
- full 接口兼容：`GET/PUT /api/inbounds/:id/full`
- hysteria2(hy2) 入站创建/更新
- hy2 UDP hop (`udphop`) 参数支持（`hy2HopPorts` / `hy2HopInterval`）
- `hy2://` 链接导出支持 `mport` / `mportInterval`

## 快速启动

- 运行：
  - `go run ./cmd/sui-go`
- 默认监听：
  - `:8811`
- 可用环境变量：
  - `ADDR`（默认 `:8811`）
  - `DB_FILE`（默认 `data/sui-go.db`）
  - `PANEL_USER`（默认 `admin`）
  - `PANEL_PASS`（默认 `admin123`）

## API（当前）

- `GET /api/health`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /api/inbounds`（需 Bearer Token）
- `POST /api/inbounds/add`（需 Bearer Token）
- `GET /api/inbounds/:id`（需 Bearer Token）
- `PUT /api/inbounds/:id`（需 Bearer Token）
- `DELETE /api/inbounds/:id`（需 Bearer Token）
- `GET /api/inbounds/:id/full`（需 Bearer Token）
- `PUT /api/inbounds/:id/full`（需 Bearer Token）
- `GET /api/inbounds/:id/links`（需 Bearer Token）
- `GET /api/xray/config`（需 Bearer Token）

示例（新增 hy2 + 端口跳跃）：

`POST /api/inbounds/add`

```json
{
  "remark": "hy2-hop-test",
  "port": 24433,
  "protocol": "hysteria",
  "password": "abc123",
  "sni": "www.bing.com",
  "hy2HopPorts": "25000-25010,25020",
  "hy2HopInterval": "20-40"
}
```

## 部署（新增）

已提供一键安装脚本与 systemd 单元模板：
- `install.sh`
- `sui-go.service`

快速安装：
- `sudo bash install.sh`

安装后常用命令：
- `systemctl status sui-go --no-pager`
- `systemctl restart sui-go`
- `journalctl -u sui-go -n 100 --no-pager`

环境变量文件：
- `/etc/default/sui-go`

## 路线图（对标 s-ui）

1. 认证增强：改密与会话策略细化
2. 协议层：补 reality/xhttp 细项与更多 streamSettings
3. 运行层：xray/sing-box 进程控制与配置生成
4. 前端层：继续补齐参数面板与协议细分表单
5. 发布层：升级脚本与版本发布流程

## License

MIT

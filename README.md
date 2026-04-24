# sui-go

Go 版本的 SUI（对标 s-ui 的 Go 架构方向），用于逐步替换现有 Node 面板。

## 当前状态

当前为 **Phase-2 迭代版**：
- Go 原生 HTTP 服务
- SQLite 持久化（gorm）
- 认证接口 `POST /auth/login`（Bearer Token 访问受保护 API）
- inbounds 基础接口（list/add/links）
- hysteria2(hy2) 入站创建
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

## API（Phase-1）

- `GET /api/health`
- `POST /auth/login`
- `GET /api/inbounds`（需 Bearer Token）
- `POST /api/inbounds/add`（需 Bearer Token）
- `GET /api/inbounds/:id/links`（需 Bearer Token）

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

## 路线图（对标 s-ui）

1. 存储层：JSON -> SQLite（gorm）
2. 认证层：token/session + 面板用户管理
3. 协议层：vless/vmess/trojan/ss/reality/xhttp 等全量迁移
4. 运行层：xray/sing-box 进程控制与配置生成
5. 前端层：迁移现有 `public/index.html` 到 Go embed + API 适配
6. 发布层：systemd + install.sh + 一键升级

## License

MIT

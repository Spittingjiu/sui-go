# sui-go

`sui-go` 是一个面向 **x-ui 使用习惯** 的 Go 版面板后端（含内置 Web UI），目标是：
- 操作体验对齐 x-ui（登录、入站管理、链接/二维码、系统操作）
- 架构升级为 Go + SQLite，降低运行复杂度
- 与 `sui-sub` 打通，形成“面板管理 + 订阅分发”闭环

> 当前仓库定位：可直接上线使用的持续迭代版（不是 demo）。

---

## 1. 对标 x-ui：能力总览

## 已对齐的核心能力
- 认证与会话：`/auth/login`、`/auth/refresh`、`/auth/logout`、`/auth/me`
- 入站全生命周期：新增/查询/更新/删除/启停/批量启停
- 入站详情扩展：`/api/inbounds/:id/full`（兼容 full 字段场景）
- 链接能力：`/api/inbounds/:id/links`、`/api/inbounds/:id/qr`
- 系统运维入口：xray 重启/切换/配置导出/状态检查
- Xray 配置链路：`/api/xray/config`、`/api/xray/export`、`/api/xray/apply`

## 已实现的增强（在对齐基础上额外补强）
- 配置缓存：`/api/xray/config` 命中缓存后更快
- apply 串行化 + 超时保护：避免并发 reload 打架
- apply 无变化跳过：配置不变时直接 `skipped=true`
- apply 失败回滚：reload 失败自动回退 last-good 配置
- apply 事件日志：`/api/xray/apply-events`
- apply 聚合统计：`/api/xray/apply-stats?sinceMin=60`
- 入站轻量列表：`/api/inbounds?full=0&limit=&offset=`（UI 默认已切换）
- 批量写入：`/api/inbounds/add-batch`（单次最多 200 条）

## 协议支持（当前）
- vless
- vmess
- trojan
- shadowsocks
- hysteria(v2)

补充：
- vless 支持 reality/xhttp 常用字段
- hy2 支持 hop 参数（`hy2HopPorts`、`hy2HopInterval`）

---

## 2. Sub 能力（重点）

`sui-go` 已支持直接对接 `sui-sub`，把当前面板节点源写入订阅系统。

## 2.1 对接接口
- `POST /api/panel/connect-sub`（需 Bearer Token）

请求字段：
- `subUrl`：sui-sub 地址（例如 `https://sub.example.com`）
- `subUsername`：sui-sub 登录用户名
- `subPassword`：sui-sub 登录密码
- `sourceName`：写入到 sui-sub 的源名称（可选，默认 `sui-go`）

返回：
- 成功时返回“已写入到 sui-sub”及目标地址信息
- 失败时返回明确的上游错误（登录失败、会话 cookie 缺失、写入失败等）

## 2.2 对接流程（服务端行为）
1. sui-go 校验输入参数
2. 登录 sui-sub，获取会话 cookie
3. 用会话调用 sui-sub 源写入接口
4. 返回结果给面板前端（成功/失败可观测）

## 2.3 前端入口
- 首页已内置 “Connect Sub” 区域（URL/用户名/密码/源名称）
- 点击后直接走 `connect-sub` 接口，不需要手工拼请求

## 2.4 典型使用场景
- 面板侧完成节点录入后，一键同步到订阅平台
- 将多个节点源统一汇聚在 sui-sub 侧做订阅出口
- 作为 x-ui 迁移阶段的过渡桥接（管理与订阅分离）

---

## 3. 快速安装

一键安装：
- `curl -fsSL https://raw.githubusercontent.com/Spittingjiu/sui-go/main/install.sh | bash`

安装后常用命令：
- `systemctl status sui-go --no-pager`
- `systemctl restart sui-go`
- `journalctl -u sui-go -n 100 --no-pager`

环境文件：
- `/etc/default/sui-go`

---

## 4. 启动配置

默认监听：
- `:18811`

环境变量：
- `ADDR`：监听地址（默认 `:18811`）
- `DB_FILE`：SQLite 文件（默认 `data/sui-go.db`）
- `PANEL_USER`：面板用户名（默认 `admin`）
- `PANEL_PASS`：面板密码（默认 `admin123`）
- `XRAY_CONFIG_OUT`：xray 配置输出路径（默认 `data/xray-config.json`）
- `XRAY_RELOAD_CMD`：xray reload 命令（为空则 apply 仅写文件不 reload）

---

## 5. API 速查（当前可用）

## 认证
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /auth/me`

## 入站
- `GET /api/inbounds`
- `POST /api/inbounds/add`
- `POST /api/inbounds/add-batch`
- `POST /api/inbounds/add-reality-quick`
- `GET /api/inbounds/next-port`
- `POST /api/inbounds/batch-toggle`
- `GET /api/inbounds/:id`
- `PUT /api/inbounds/:id`
- `DELETE /api/inbounds/:id`
- `POST /api/inbounds/:id/toggle`
- `GET /api/inbounds/:id/full`
- `PUT /api/inbounds/:id/full`
- `GET /api/inbounds/:id/links`
- `GET /api/inbounds/:id/qr`

## Xray
- `GET /api/xray/config`
- `POST /api/xray/export`
- `POST /api/xray/apply`
- `GET /api/xray/apply-events`
- `GET /api/xray/apply-stats`

## 转发
- `GET/POST /api/forwards`
- `PUT/DELETE /api/forwards/:id`
- `POST /api/forwards/:id/toggle`

## 面板设置与 Sub
- `GET/POST /api/panel/settings`
- `GET /api/panel/token`
- `POST /api/panel/token/rotate`
- `POST /api/panel/change-password`
- `POST /api/panel/connect-sub`

## 系统
- `GET /api/system/status`
- `POST /api/system/restart-panel`
- `POST /api/system/update-panel`
- `POST /api/system/chain/test`
- `POST /api/system/restart-xray`
- `POST /api/system/restart-xui`
- `POST /api/system/optimize/bbr`
- `POST /api/system/optimize/dns`
- `POST /api/system/optimize/sysctl`
- `POST /api/system/optimize/all`
- `GET /api/system/xray/version-current`
- `GET /api/system/xray/reality-gen`
- `GET/POST /api/system/xray/config`
- `GET /api/system/xray/versions`
- `POST /api/system/xray/switch`

## 视图初始化
- `GET /api/view/bootstrap`

---

## 6. 性能与稳定性（最近迭代）

已落地：
- 入站列表 lite 查询默认化（UI）
- apply 事件日志 + 统计接口
- apply 失败自动回滚
- SQLite 索引（port/protocol/enable）
- SQLite WAL + 连接池参数调优
- 批量写入接口与基准脚本

仓库内可参考：
- `docs/perf-scale-report-2026-04-25.json`
- `scripts/perf-smoke.sh`
- `scripts/perf-smoke-concurrency.sh`
- `scripts/fault-inject-smoke.sh`
- `scripts/write-batch-benchmark.sh`
- `docs/xui-parity-protocol-checklist-2026-04-25.md`

---

## 7. 部署文件

- `install.sh`：安装脚本
- `sui-go.service`：systemd 服务模板

---

## 8. 路线图（继续对标 x-ui）

- 完善协议高级参数覆盖率（按 x-ui 参数矩阵持续补齐）
- 强化导入导出与迁移工具链
- 前端交互继续贴近 x-ui 常用路径
- 扩展 sub 协同能力（源管理、同步审计、重试策略）

---

## 9. License

MIT

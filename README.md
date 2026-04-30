# sui-go

<p align="center">
  <b>一块安静、利落、能真正上线的 Xray 管理面板。</b><br />
  Go 后端 · SQLite 存储 · Apple 风 Web UI · sui-sub 桥接 · VPS 命令行菜单
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
  <img alt="SQLite" src="https://img.shields.io/badge/SQLite-local-003B57?style=for-the-badge&logo=sqlite&logoColor=white" />
  <img alt="Xray" src="https://img.shields.io/badge/Xray-ready-111111?style=for-the-badge" />
  <img alt="License" src="https://img.shields.io/badge/License-MIT-black?style=for-the-badge" />
</p>

---

## 这是什么

`sui-go` 是一个轻量、克制、偏工程实用主义的代理面板。

它不是为了把页面做成控制中心，也不是为了塞满按钮证明自己很强。它的目标很简单：

- 打开就能看到节点。
- 一键就能创建常用节点。
- 复制、二维码、编辑、删除都在同一个工作流里。
- 面板设置、Xray 更新、sui-sub 对接这些低频能力，收进该待的地方。
- 后端用 Go + SQLite，部署干净，迁移简单，跑在小 VPS 上也不累。

一句话：**把代理面板从“功能堆叠”拉回“真实使用”。**

---

## ✨ 亮点

### 少入口，高效率

默认首页就是节点列表。没有多余首页，没有仪表盘噪音，没有让人迷路的菜单。

当前主导航只有：

- **节点**：创建、复制、扫码、编辑、删除。
- **概览 SUB**：系统概览与 sui-sub 对接。
- **面板设置**：账号、安全、Xray 更新、配置导出与 reload。

### 一键创建常用节点

内置极速创建：

- 一键 HY2
- 一键 Reality
- 一键 Trojan

Reality 默认目标使用 `www.icloud.com:443`，并提供来自 `sui` 最新 GitHub 版本的目标域名池：

- `www.icloud.com`
- `www.lovelive-anime.jp`
- `addons.mozilla.org`
- `www.microsoft.com`
- `www.apple.com`
- `www.bing.com`
- `www.amazon.com`

### Xray 更新更清楚

Xray 更新不再只给一坨技术输出。

面板会展示：

- 当前版本
- 稳定版最新
- 开发版最新
- 当前选择通道是否需要更新

并支持：

- 检测稳定版 / 开发版
- 更新到所选版本
- 过滤 prerelease 的稳定版口径

### sui-sub 一键桥接

`sui-go` 可以把当前面板节点源写入 `sui-sub`，形成：

> 面板录入节点 → sui-sub 聚合规则 → 客户端订阅分发

不用手工拼请求，也不用在两个系统之间来回复制。

### VPS 里输入 `sui` 就能管理

安装后提供命令行菜单：

```text
sui
```

菜单包含：

1. 修改面板账号密码
2. 显示当前用户信息
3. 修改面板端口
4. 启用 BBR + fq
5. 一键对接 sub
6. 更新 sui-go 面板
7. Xray 更新（稳定版 / 开发版）
8. 一键卸载 sui-go

---

## 快速安装

```bash
curl -fsSL https://raw.githubusercontent.com/Spittingjiu/sui-go/main/install.sh | bash
```

默认监听：

```text
http://服务器IP:18811
```

默认账号：

```text
用户名：admin
密码：admin123
```

> 上线后建议第一时间在「面板设置」里修改用户名和密码。

---

## 常用命令

```bash
# 打开 VPS 命令行菜单
sui

# 查看服务状态
systemctl status sui-go --no-pager

# 重启面板
systemctl restart sui-go

# 查看日志
journalctl -u sui-go -n 100 --no-pager
```

---

## 环境配置

配置文件：

```text
/etc/default/sui-go
```

常用环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ADDR` | `:18811` | 面板监听地址 |
| `DB_FILE` | `/opt/sui-go/data/sui-go.db` | SQLite 数据库路径 |
| `PANEL_USER` | `admin` | 初始面板用户名 |
| `PANEL_PASS` | `admin123` | 初始面板密码 |
| `XRAY_CONFIG_OUT` | `/usr/local/etc/xray/config.json` | Xray 配置导出路径 |
| `XRAY_RELOAD_CMD` | `systemctl restart xray` | 导出配置后的 reload 命令 |

---

## 支持能力

### 节点协议

当前支持：

- VLESS
- VMess
- Trojan
- Shadowsocks
- Hysteria2 / HY2
- SOCKS
- HTTP
- Dokodemo-door
- WireGuard
- TUN
- Port Forward

其中：

- VLESS 支持 Reality、TLS、WS、XHTTP、HTTPUpgrade、gRPC 等常用组合。
- HY2 支持端口跳跃参数。
- UUID、密码、端口、Reality 参数等高频字段可自动生成。

### 面板能力

- 登录 / 退出 / 会话校验
- 节点新增、编辑、删除、启停
- 轻量节点列表，首屏更快
- 节点链接与二维码
- 面板用户名 / 密码修改
- Xray 配置导出与 reload
- Xray 稳定版 / 开发版更新
- BBR + fq 优化入口
- sui-sub 对接

### 稳定性设计

- Go 单二进制部署
- SQLite 本地存储
- 配置导出支持 last-good 回滚
- apply 操作串行化，避免并发 reload 打架
- apply 事件日志与统计接口
- 本地运行数据默认不纳入 Git

---

## API 速查

### 认证

- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /auth/me`

### Inbounds

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

### Port Forward

- `GET /api/forwards`
- `POST /api/forwards`
- `PUT /api/forwards/:id`
- `DELETE /api/forwards/:id`
- `POST /api/forwards/:id/toggle`

### Xray 配置

- `GET /api/xray/config`
- `POST /api/xray/export`
- `POST /api/xray/apply`
- `GET /api/xray/apply-events`
- `GET /api/xray/apply-stats`

### 面板设置与 sui-sub

- `GET /api/panel/settings`
- `POST /api/panel/settings`
- `GET /api/panel/token`
- `POST /api/panel/token/rotate`
- `POST /api/panel/change-username`
- `POST /api/panel/change-password`
- `POST /api/panel/connect-sub`

### 系统

- `GET /api/system/status`
- `POST /api/system/restart-panel`
- `POST /api/system/update-panel`
- `POST /api/system/chain/test`
- `POST /api/system/restart-xray`
- `POST /api/system/optimize/bbr`
- `POST /api/system/optimize/dns`
- `POST /api/system/optimize/sysctl`
- `POST /api/system/optimize/all`
- `GET /api/system/xray/version-current`
- `GET /api/system/xray/reality-gen`
- `GET /api/system/xray/config`
- `POST /api/system/xray/config`
- `GET /api/system/xray/versions`
- `POST /api/system/xray/switch`

### 视图初始化

- `GET /api/view/bootstrap`

---

## 本地开发

```bash
git clone https://github.com/Spittingjiu/sui-go.git
cd sui-go

go test ./...
go build -o sui-go ./cmd/sui-go

ADDR=:18811 ./sui-go
```

打开：

```text
http://127.0.0.1:18811
```

---

## 项目结构

```text
sui-go/
├── cmd/sui-go/          # 程序入口
├── internal/app/        # HTTP API 与业务逻辑
├── internal/model/      # 数据模型
├── internal/store/      # SQLite 存储层
├── public/              # 内置 Web UI
├── scripts/             # 测试与辅助脚本
├── docs/                # 性能、协议矩阵等文档
├── install.sh           # 一键安装脚本
└── sui-go.service       # systemd 服务模板
```

---

## 设计取向

`sui-go` 的审美不是「赛博控制台」，而是「工具应该消失在任务背后」。

所以它会尽量：

- 少一点装饰，多一点路径清晰。
- 少一点跳转，多一点就地完成。
- 少一点吓人的错误，多一点能继续操作的提示。
- 少一点“看起来很强”，多一点“真的好用”。

---

## 路线图

- 继续补齐协议高级参数覆盖率
- 强化导入 / 导出 / 迁移工具链
- 完善 sui-sub 协同能力：源管理、同步审计、失败重试
- 增加更完整的部署形态文档：直连、反代、HTTPS
- 持续压缩高频操作路径，让面板更像工具，而不是迷宫

---

## License

MIT

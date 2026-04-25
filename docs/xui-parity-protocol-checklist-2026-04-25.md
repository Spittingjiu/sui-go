# x-ui 协议对标清单（2026-04-25）

> 范围：仅看“协议构建/参数能力”，不含流量统计、用户管理、前端视觉。

## A. 协议覆盖
- [x] hysteria(hy2)
- [x] vless
- [x] vmess
- [x] trojan
- [x] shadowsocks
- [x] socks
- [x] http
- [x] dokodemo-door
- [x] wireguard
- [x] tun

## B. 新增节点自动生成能力（对标 x-ui 交互习惯）
- [x] port 自动分配（空值时）
- [x] remark 自动生成（`协议-端口`）
- [x] vmess/vless UUID 自动生成
- [x] trojan/hy2/ss password 自动生成
- [x] vless+reality 自动生成 x25519 keypair / shortId / 默认 dest
- [x] sniffing 默认启用 + 可覆盖

## C. 已补齐的关键参数（本轮）
- [x] HY2: `obfs` / `obfsPassword` / `congestion`
- [x] HY2: `up_mbps` / `down_mbps`
- [x] HY2: `keepAlivePeriod`
- [x] HY2: stream/connection receive window 4 项
- [x] HY2: `disablePathMTUDiscovery`
- [x] settings/stream 通用覆盖能力（`settingsOverride`/`streamOverride` 深度合并）

## D. 仍需继续补齐（下一轮）
### D1. 协议参数细节
- [x] vless/vmess/trojan：补更细 TLS 配置项（ALPN、cipher suites、min/max version、fingerprint）
- [x] vless/vmess/trojan：补 KCP 细项、gRPC 多字段、xhttp 更细模式项
- [ ] shadowsocks：补多用户/2022 系列方法细粒度校验
- [ ] socks/http：补多账号输入与校验策略
- [ ] wireguard：补 peers 全量字段（preSharedKey/keepAlive 多 peer）
- [ ] tun：补与 x-ui 一致的可配字段集合（当前仅核心项）

### D2. 参数校验与安全
- [ ] 按协议建立字段白名单与类型约束（减少 override 误配）
- [x] 端口冲突/保留端口校验
- [x] 关键参数格式校验（uuid/shortId/sni/path 等）

### D3. 实测矩阵（持续）
- [ ] 参数级矩阵（不仅协议+传输，还含高风险参数组合）
- [ ] 每轮产出机读报告并入库 docs/

## E. 本轮结论
- 协议覆盖已全；新增节点自动生成能力已达标。
- 本轮新增 HY2 高级参数与 override 合并能力，`xray run -test` 已通过。
- 下一轮进入“参数细粒度对标 + 参数级矩阵实测”。

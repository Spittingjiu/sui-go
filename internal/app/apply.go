package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// applyXrayConfig 把当前数据库里的节点真正写成 Xray 配置并重载服务。
// 之前「一键创建」只把节点写进 SQLite，从不落盘/重载，导致面板里
// 节点看着建好了，实际 Xray 里没有对应 inbound、端口没监听、客户端必然连不上。
func (a *App) applyXrayConfig() error {
	cfg, err := a.buildXrayConfig()
	if err != nil {
		return fmt.Errorf("构建 Xray 配置失败: %w", err)
	}
	// 只有存在 hysteria/hy2 节点时才需要自签证书，避免无 openssl 的机器卡住应用。
	if needHY2Certificate(cfg) {
		if err := ensureHY2CertificateFiles(); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	out := strings.TrimSpace(a.cfg.XrayConfigOut)
	if out == "" {
		out = "data/xray-config.json"
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	now := time.Now().Unix()
	existing, _ := os.ReadFile(out)
	_ = existing
	if bytes.Equal(existing, b) {
		a.appendApplyEvent(map[string]any{"ts": now, "success": true, "applied": false, "skipped": true, "reason": "unchanged"})
		return nil
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return err
	}
	if strings.TrimSpace(a.cfg.XrayReloadCmd) == "" {
		a.appendApplyEvent(map[string]any{"ts": now, "success": true, "applied": false, "reason": "reload_not_configured"})
		return nil
	}
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	reloadOut, err := exec.CommandContext(ctx, "bash", "-lc", a.cfg.XrayReloadCmd).CombinedOutput()
	if err != nil {
		_ = os.WriteFile(out+".last-good", existing, 0o644)
		a.appendApplyEvent(map[string]any{"ts": now, "success": false, "error": err.Error(), "output": strings.TrimSpace(string(reloadOut))})
		return fmt.Errorf("Xray 重载失败: %v: %s", err, strings.TrimSpace(string(reloadOut)))
	}
	a.appendApplyEvent(map[string]any{"ts": now, "success": true, "applied": true})
	return nil
}

// needHY2Certificate 判断配置里是否有需要自签证书的节点。
func needHY2Certificate(cfg map[string]any) bool {
	ibs, _ := cfg["inbounds"].([]map[string]any)
	for _, ib := range ibs {
		proto, _ := ib["protocol"].(string)
		if strings.EqualFold(proto, "hysteria") || strings.EqualFold(proto, "hysteria2") {
			return true
		}
		ss, _ := ib["streamSettings"].(map[string]any)
		if ss != nil {
			if sec, _ := ss["security"].(string); strings.EqualFold(sec, "tls") {
				if _, ok := ss["tlsSettings"].(map[string]any); ok {
					return true
				}
			}
		}
	}
	return false
}

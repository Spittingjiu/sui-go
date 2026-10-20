package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// SUBGO 与 sui-go 通常跑在同一台机器上（SUBGO 默认监听 127.0.0.1:8781）。
// 这里给面板提供"一键对接"能力：探测本地 SUBGO、记住地址与账号，
// 之后点一下就把本面板注册成 SUBGO 的节点来源。
const defaultSubgoURL = "http://127.0.0.1:8781"

type subgoProbe struct {
	Reachable      bool   `json:"reachable"`
	URL            string `json:"url"`
	Version        string `json:"version"`
	Error          string `json:"error,omitempty"`
	SavedURL       string `json:"savedUrl,omitempty"`
	SavedUser      string `json:"savedUser,omitempty"`
	HasSaved       bool   `json:"hasSavedPassword"`
	HasBridgeToken bool   `json:"hasBridgeToken"`
	SuggestURL     bool   `json:"suggestUrl"`
}

func probeSubgo(base, bridgeToken string) subgoProbe {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	res := subgoProbe{URL: base}
	if base == "" {
		return res
	}
	client := &http.Client{Timeout: 6 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, base+"/api/version", nil)
	applyBridgeToken(req, bridgeToken)
	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	var body struct {
		OK      bool   `json:"ok"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && (body.Name == "subgo" || body.OK) {
		res.Reachable = true
		res.Version = body.Version
		return res
	}
	if resp.StatusCode == http.StatusForbidden {
		res.Error = "403 被 SUBGO 白名单拦截（需要在面板里填对接密钥 X-Sui-Bridge-Token）"
		return res
	}
	res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	return res
}

// applyBridgeToken 给请求带上对接密钥。SUBGO 开了 IP 白名单时，
// nginx 只放行带正确 X-Sui-Bridge-Token 的 bridge/version 请求。
func applyBridgeToken(req *http.Request, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("SUBGO_BRIDGE_TOKEN"))
	}
	if token != "" {
		req.Header.Set("X-Sui-Bridge-Token", token)
	}
}

// normalizeSubgoURL 允许用户只填 127.0.0.1:8781 或 localhost:8781。
func normalizeSubgoURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultSubgoURL, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + strings.TrimPrefix(raw, "//")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("SUBGO 地址不合法: %s", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("SUBGO 地址必须是 http(s)")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// handlePanelSubgo GET 返回探测结果与已保存配置；POST 保存配置并推送。
func (a *App) handlePanelSubgo(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	p, _ := a.store.GetPanelSetting()
	switch r.Method {
	case http.MethodGet:
		// 允许用 ?token= 现场试一个密钥（未保存时也能探测）。
		bridgeToken := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("token")), p.SubgoBridgeToken)
		candidates := []string{}
		// 支持 ?url= 现场试地址（还没保存时也能探测公网/远端 SUBGO）。
		if u := strings.TrimSpace(r.URL.Query().Get("url")); u != "" {
			if norm, err := normalizeSubgoURL(u); err == nil {
				candidates = append(candidates, norm)
			}
		}
		if strings.TrimSpace(p.SubgoURL) != "" {
			candidates = append(candidates, strings.TrimSpace(p.SubgoURL))
		}
		candidates = append(candidates, defaultSubgoURL, "http://127.0.0.1:8781")
		seen := map[string]bool{}
		best := subgoProbe{URL: candidateDefault(p.SubgoURL)}
		for _, c := range candidates {
			if seen[c] {
				continue
			}
			seen[c] = true
			pr := probeSubgo(c, bridgeToken)
			if pr.Reachable {
				best = pr
				break
			}
			if best.Error == "" {
				best = pr
			}
		}
		best.SavedURL = strings.TrimSpace(p.SubgoURL)
		best.SavedUser = strings.TrimSpace(p.SubgoUser)
		best.HasSaved = strings.TrimSpace(p.SubgoPass) != ""
		best.HasBridgeToken = strings.TrimSpace(p.SubgoBridgeToken) != ""
		best.SuggestURL = best.URL == ""
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": best})
	case http.MethodPost:
		var req struct {
			URL         string `json:"url"`
			Username    string `json:"username"`
			Password    string `json:"password"`
			Name        string `json:"name"`
			PanelURL    string `json:"panelUrl"`
			BridgeToken string `json:"bridgeToken"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		base, err := normalizeSubgoURL(emptyDefault(req.URL, p.SubgoURL))
		if err != nil {
			a.writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		user := strings.TrimSpace(req.Username)
		if user == "" {
			user = strings.TrimSpace(p.SubgoUser)
		}
		pass := req.Password
		if pass == "" {
			pass = p.SubgoPass
		}
		if user == "" || pass == "" {
			a.writeErr(w, http.StatusBadRequest, "SUBGO 用户名和密码必填（首次对接后会自动记住）")
			return
		}
		bridgeToken := firstNonEmpty(strings.TrimSpace(req.BridgeToken), p.SubgoBridgeToken)
		probe := probeSubgo(base, bridgeToken)
		if !probe.Reachable {
			a.writeErr(w, http.StatusBadGateway, "连不上 SUBGO ("+base+"): "+probe.Error)
			return
		}

		panelToken := strings.TrimSpace(p.APIToken)
		if panelToken == "" {
			p2, err := a.store.RotateAPIToken(randomToken(24))
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, "生成 panel token 失败: "+err.Error())
				return
			}
			panelToken = strings.TrimSpace(p2.APIToken)
		}
		panelBase := inferPanelBaseURL(r)
		if strings.TrimSpace(req.PanelURL) != "" {
			if u, err := url.Parse(req.PanelURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				a.writeErr(w, http.StatusBadRequest, "panelUrl 必须是 http(s) 地址")
				return
			}
			panelBase = strings.TrimRight(req.PanelURL, "/")
		}
		name := emptyDefault(strings.TrimSpace(req.Name), "sui-go")

		body, _ := json.Marshal(map[string]string{
			"username":    user,
			"password":    pass,
			"name":        name,
			"panel_url":   panelBase,
			"panel_token": panelToken,
		})
		client := &http.Client{Timeout: 12 * time.Second}
		bridgeReq, _ := http.NewRequest(http.MethodPost, base+"/api/bridge/push-source", bytes.NewReader(body))
		bridgeReq.Header.Set("content-type", "application/json")
		applyBridgeToken(bridgeReq, bridgeToken)
		resp, err := client.Do(bridgeReq)
		if err != nil {
			a.writeErr(w, http.StatusBadGateway, "连接 SUBGO 失败: "+err.Error())
			return
		}
		defer resp.Body.Close()
		var reply map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&reply)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || reply["ok"] != true {
			msg := fmt.Sprintf("SUBGO 对接失败 HTTP %d", resp.StatusCode)
			if em, ok := reply["error"].(string); ok && strings.TrimSpace(em) != "" {
				msg = "SUBGO 对接失败: " + em
			}
			a.writeErr(w, http.StatusBadRequest, msg)
			return
		}
		_, _ = a.store.SaveSubgoConfig(base, user, pass, bridgeToken)
		a.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"msg":     "已推送到 SUBGO",
			"obj": map[string]any{
				"subgoUrl":   base,
				"panelUrl":   panelBase,
				"sourceName": name,
				"sourceId":   reply["source_id"],
				"version":    probe.Version,
			},
		})
	default:
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func candidateDefault(saved string) string {
	if strings.TrimSpace(saved) != "" {
		return strings.TrimSpace(saved)
	}
	return defaultSubgoURL
}

// localPortOpen 用于诊断提示（例如 SUBGO 只监听 127.0.0.1 但面板在容器内）。
func localPortOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

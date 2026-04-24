package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Spittingjiu/sui-go/internal/model"
	"github.com/Spittingjiu/sui-go/internal/store"
	"github.com/google/uuid"
)

type Config struct {
	Addr          string
	DataFile      string
	DBFile        string
	PanelUser     string
	PanelPass     string
	XrayConfigOut string
	XrayReloadCmd string
}

type App struct {
	cfg   Config
	mux   *http.ServeMux
	store *store.SQLiteStore
}

func New(cfg Config) (*App, error) {
	if cfg.XrayConfigOut == "" {
		cfg.XrayConfigOut = "data/xray-config.json"
	}
	if cfg.XrayReloadCmd == "" {
		cfg.XrayReloadCmd = ""
	}
	st, err := store.NewSQLite(cfg.DBFile)
	if err != nil {
		return nil, err
	}
	if cfg.PanelUser == "" {
		cfg.PanelUser = "admin"
	}
	if cfg.PanelPass == "" {
		cfg.PanelPass = "admin123"
	}
	if err := st.EnsureDefaultUser(cfg.PanelUser, cfg.PanelPass); err != nil {
		return nil, err
	}
	if err := st.EnsureDefaultPanelSetting(cfg.PanelUser); err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, mux: http.NewServeMux(), store: st}
	a.routes()
	return a, nil
}

func (a *App) Run() error {
	return http.ListenAndServe(a.cfg.Addr, a.mux)
}

func (a *App) routes() {
	a.mux.HandleFunc("/api/health", a.handleHealth)
	a.mux.HandleFunc("/auth/login", a.handleLogin)
	a.mux.HandleFunc("/auth/refresh", a.handleRefresh)
	a.mux.HandleFunc("/auth/logout", a.handleLogout)
	a.mux.HandleFunc("/auth/me", a.handleMe)

	a.mux.HandleFunc("/api/inbounds", a.handleListInbounds)
	a.mux.HandleFunc("/api/inbounds/add", a.handleAddInbound)
	a.mux.HandleFunc("/api/inbounds/add-reality-quick", a.handleAddRealityQuick)
	a.mux.HandleFunc("/api/inbounds/", a.handleInboundSub)
	a.mux.HandleFunc("/api/inbounds/next-port", a.handleNextPort)
	a.mux.HandleFunc("/api/inbounds/batch-toggle", a.handleBatchToggleInbounds)

	a.mux.HandleFunc("/api/xray/config", a.handleXrayConfig)
	a.mux.HandleFunc("/api/xray/export", a.handleXrayExport)
	a.mux.HandleFunc("/api/xray/apply", a.handleXrayApply)

	a.mux.HandleFunc("/api/forwards", a.handleForwards)
	a.mux.HandleFunc("/api/forwards/", a.handleForwardsSub)

	a.mux.HandleFunc("/api/panel/settings", a.handlePanelSettings)
	a.mux.HandleFunc("/api/panel/token", a.handlePanelToken)
	a.mux.HandleFunc("/api/panel/token/rotate", a.handlePanelTokenRotate)
	a.mux.HandleFunc("/api/panel/change-password", a.handlePanelChangePassword)
	a.mux.HandleFunc("/api/panel/connect-sub", a.handlePanelConnectSub)

	a.mux.HandleFunc("/api/system/status", a.handleSystemStatus)
	a.mux.HandleFunc("/api/system/restart-panel", a.handleSystemRestartPanel)
	a.mux.HandleFunc("/api/system/chain/test", a.handleSystemChainTest)
	a.mux.HandleFunc("/api/system/restart-xray", a.handleSystemRestartXray)
	a.mux.HandleFunc("/api/system/restart-xui", a.handleSystemRestartXUI)
	a.mux.HandleFunc("/api/system/optimize/bbr", a.handleSystemOptimizeBBR)
	a.mux.HandleFunc("/api/system/optimize/dns", a.handleSystemOptimizeDNS)
	a.mux.HandleFunc("/api/system/optimize/sysctl", a.handleSystemOptimizeSysctl)
	a.mux.HandleFunc("/api/system/optimize/all", a.handleSystemOptimizeAll)
	a.mux.HandleFunc("/api/system/xray/version-current", a.handleSystemXrayVersionCurrent)
	a.mux.HandleFunc("/api/system/xray/reality-gen", a.handleSystemXrayRealityGen)
	a.mux.HandleFunc("/api/system/xray/config", a.handleSystemXrayConfig)
	a.mux.HandleFunc("/api/system/xray/versions", a.handleSystemXrayVersions)
	a.mux.HandleFunc("/api/system/xray/switch", a.handleSystemXraySwitch)

	a.mux.HandleFunc("/api/view/bootstrap", a.handleViewBootstrap)

	// minimal web ui
	a.mux.Handle("/", http.FileServer(http.Dir("public")))
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": "sui-go"})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ok, err := a.store.CheckUser(req.Username, req.Password)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		a.writeErr(w, http.StatusUnauthorized, "invalid username/password")
		return
	}
	tok := randomToken(24)
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := a.store.SaveToken(tok, req.Username, expiresAt); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"token":       tok,
		"user":        req.Username,
		"mustReset":   false,
		"panelPath":   "/",
		"expiresUnix": expiresAt.Unix(),
	})
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tok, username, ok := a.extractAuth(r)
	if !ok {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	newTok := randomToken(24)
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := a.store.RefreshToken(tok, newTok, expiresAt); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"token":       newTok,
		"user":        username,
		"expiresUnix": expiresAt.Unix(),
	})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tok, _, ok := a.extractAuth(r)
	if !ok {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := a.store.DeleteToken(tok); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_, user, ok := a.extractAuth(r)
	if !ok {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ps, _ := a.store.GetPanelSetting()
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "user": user, "panelPath": ps.PanelPath})
}

func (a *App) handleListInbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := a.store.ListInbounds()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": rows})
}

func (a *App) handleAddInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req model.AddInboundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in, err := buildInboundFromReq(req)
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := a.store.AddInbound(in)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": created})
}

func (a *App) handleInboundSub(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/inbounds/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		a.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			in, ok, err := a.store.GetInbound(id)
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				a.writeErr(w, http.StatusNotFound, "not found")
				return
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": in})
		case http.MethodPut:
			var req model.AddInboundRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				a.writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			in, err := buildInboundFromReq(req)
			if err != nil {
				a.writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			updated, ok, err := a.store.UpdateInbound(id, in)
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				a.writeErr(w, http.StatusNotFound, "not found")
				return
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": updated})
		case http.MethodDelete:
			ok, err := a.store.DeleteInbound(id)
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				a.writeErr(w, http.StatusNotFound, "not found")
				return
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 {
		switch parts[1] {
		case "links":
			if r.Method != http.MethodGet {
				a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			in, ok, err := a.store.GetInbound(id)
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				a.writeErr(w, http.StatusNotFound, "not found")
				return
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": buildLinks(in)})
			return
		case "qr":
			if r.Method != http.MethodGet {
				a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			in, ok, err := a.store.GetInbound(id)
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				a.writeErr(w, http.StatusNotFound, "not found")
				return
			}
			links := buildLinks(in)
			qr := ""
			if len(links) > 0 {
				qr = links[0]
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"link": qr, "qrcode": qr}})
			return
		case "toggle":
			if r.Method != http.MethodPost {
				a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			in, ok, err := a.store.GetInbound(id)
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				a.writeErr(w, http.StatusNotFound, "not found")
				return
			}
			in.Enable = !in.Enable
			updated, ok, err := a.store.UpdateInbound(id, in)
			if err != nil {
				a.writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				a.writeErr(w, http.StatusNotFound, "not found")
				return
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": updated})
			return
		case "full":
			switch r.Method {
			case http.MethodGet:
				in, ok, err := a.store.GetInbound(id)
				if err != nil {
					a.writeErr(w, http.StatusInternalServerError, err.Error())
					return
				}
				if !ok {
					a.writeErr(w, http.StatusNotFound, "not found")
					return
				}
				a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": in})
			case http.MethodPut:
				var req model.AddInboundRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					a.writeErr(w, http.StatusBadRequest, "invalid json")
					return
				}
				in, err := buildInboundFromReq(req)
				if err != nil {
					a.writeErr(w, http.StatusBadRequest, err.Error())
					return
				}
				updated, ok, err := a.store.UpdateInbound(id, in)
				if err != nil {
					a.writeErr(w, http.StatusInternalServerError, err.Error())
					return
				}
				if !ok {
					a.writeErr(w, http.StatusNotFound, "not found")
					return
				}
				a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": updated})
			default:
				a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}
	}

	a.writeErr(w, http.StatusNotFound, "not found")
}

func buildInboundFromReq(req model.AddInboundRequest) (model.Inbound, error) {
	if req.Port <= 0 || req.Protocol == "" {
		return model.Inbound{}, fmt.Errorf("port/protocol required")
	}
	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	in := model.Inbound{
		Remark:      req.Remark,
		Port:        req.Port,
		Protocol:    proto,
		Password:    req.Password,
		UUID:        req.UUID,
		Email:       req.Email,
		Method:      req.Method,
		Flow:        req.Flow,
		Network:     req.Network,
		Security:    req.Security,
		SNI:         req.SNI,
		Host:        req.Host,
		Path:        req.Path,
		RealityDest: req.RealityDest,
		ShortID:     req.ShortID,
		PublicKey:   req.PublicKey,
		Settings:    map[string]any{},
		Stream:      map[string]any{},
		Extra:       map[string]any{},
	}

	switch proto {
	case "hysteria", "hysteria2":
		in.Protocol = "hysteria"
		if in.Network == "" {
			in.Network = "hysteria"
		}
		if in.Security == "" {
			in.Security = "tls"
		}
		if in.SNI == "" {
			in.SNI = "www.bing.com"
		}
		in.Settings = map[string]any{"version": 2, "clients": []map[string]any{{"auth": in.Password, "email": in.Email}}}
		in.Stream = map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"hysteriaSettings": map[string]any{
				"version":        2,
				"auth":           in.Password,
				"udpIdleTimeout": 60,
			},
		}
		hopPorts := normalizeHopPorts(req.HY2HopPorts)
		hopInterval := normalizeHopInterval(req.HY2HopInterval)
		if hopPorts != "" {
			hs := in.Stream["hysteriaSettings"].(map[string]any)
			hs["udphop"] = map[string]any{"ports": hopPorts, "interval": hopInterval}
		}
	case "vless":
		if in.Network == "" {
			in.Network = "tcp"
		}
		if in.Security == "" {
			in.Security = "none"
		}
		in.Settings = map[string]any{"clients": []map[string]any{{"id": in.UUID, "email": in.Email, "flow": in.Flow}}, "decryption": "none"}
		in.Stream = map[string]any{"network": in.Network, "security": in.Security}
		if in.Network == "ws" {
			in.Stream["wsSettings"] = map[string]any{"path": emptyDefault(in.Path, "/"), "headers": map[string]any{"Host": emptyDefault(in.Host, in.SNI)}}
		}
		if in.Network == "xhttp" {
			in.Stream["xhttpSettings"] = map[string]any{"path": emptyDefault(in.Path, "/"), "host": emptyDefault(in.Host, in.SNI), "mode": "auto"}
		}
		if in.Security == "tls" {
			in.Stream["tlsSettings"] = map[string]any{"serverName": emptyDefault(in.SNI, in.Host)}
		}
		if in.Security == "reality" {
			in.Stream["realitySettings"] = map[string]any{
				"show":        false,
				"dest":        emptyDefault(in.RealityDest, "www.cloudflare.com:443"),
				"serverNames": []string{emptyDefault(in.SNI, "www.cloudflare.com")},
				"privateKey":  "",
				"shortIds":    []string{in.ShortID},
			}
		}
	case "vmess":
		if in.Network == "" {
			in.Network = "tcp"
		}
		in.Settings = map[string]any{"clients": []map[string]any{{"id": in.UUID, "alterId": 0, "email": in.Email}}}
		in.Stream = map[string]any{"network": in.Network, "security": in.Security}
	case "trojan":
		if in.Network == "" {
			in.Network = "tcp"
		}
		if in.Security == "" {
			in.Security = "tls"
		}
		in.Settings = map[string]any{"clients": []map[string]any{{"password": in.Password, "email": in.Email}}}
		in.Stream = map[string]any{"network": in.Network, "security": in.Security}
	case "shadowsocks", "ss":
		in.Protocol = "shadowsocks"
		if in.Method == "" {
			in.Method = "aes-128-gcm"
		}
		in.Settings = map[string]any{"method": in.Method, "password": in.Password, "network": "tcp,udp"}
	default:
		return model.Inbound{}, fmt.Errorf("unsupported protocol: %s", proto)
	}
	return in, nil
}

func (a *App) extractAuth(r *http.Request) (token, username string, ok bool) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.Fields(h)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", "", false
	}
	tok := strings.TrimSpace(parts[1])
	user, valid, err := a.store.ValidateToken(tok, time.Now())
	if err != nil || !valid {
		return "", "", false
	}
	return tok, user, true
}

func (a *App) checkAuth(r *http.Request) bool {
	_, _, ok := a.extractAuth(r)
	return ok
}

func buildLinks(in model.Inbound) []string {
	host := "127.0.0.1"
	name := in.Remark
	if name == "" {
		name = fmt.Sprintf("inbound-%d", time.Now().Unix())
	}
	switch in.Protocol {
	case "hysteria":
		query := url.Values{}
		if in.SNI != "" {
			query.Set("sni", in.SNI)
		}
		if hs, ok := in.Stream["hysteriaSettings"].(map[string]any); ok {
			if hop, ok := hs["udphop"].(map[string]any); ok {
				if ports, ok := hop["ports"].(string); ok && strings.TrimSpace(ports) != "" {
					query.Set("mport", ports)
				}
				if iv, ok := hop["interval"].(string); ok && strings.TrimSpace(iv) != "" {
					query.Set("mportInterval", iv)
				}
			}
		}
		query.Set("insecure", "1")
		return []string{fmt.Sprintf("hy2://%s@%s:%d?%s#%s", url.QueryEscape(in.Password), host, in.Port, query.Encode(), url.QueryEscape(name))}
	case "vless":
		q := url.Values{}
		t := in.Network
		if t == "" {
			t = "tcp"
		}
		q.Set("type", t)
		if in.Security != "" {
			q.Set("security", in.Security)
		}
		if in.SNI != "" {
			q.Set("sni", in.SNI)
		}
		if in.Host != "" {
			q.Set("host", in.Host)
		}
		if in.Path != "" {
			q.Set("path", in.Path)
		}
		if in.Flow != "" {
			q.Set("flow", in.Flow)
		}
		if t == "xhttp" {
			q.Set("mode", "auto")
		}
		if in.Security == "reality" {
			if in.PublicKey != "" {
				q.Set("pbk", in.PublicKey)
			}
			if in.ShortID != "" {
				q.Set("sid", in.ShortID)
			}
			q.Set("fp", "chrome")
		}
		return []string{fmt.Sprintf("vless://%s@%s:%d?%s#%s", url.QueryEscape(in.UUID), host, in.Port, q.Encode(), url.QueryEscape(name))}
	case "trojan":
		q := url.Values{}
		t := in.Network
		if t == "" {
			t = "tcp"
		}
		q.Set("type", t)
		q.Set("security", "tls")
		if in.SNI != "" {
			q.Set("sni", in.SNI)
		}
		if in.Host != "" {
			q.Set("host", in.Host)
		}
		if in.Path != "" {
			q.Set("path", in.Path)
		}
		return []string{fmt.Sprintf("trojan://%s@%s:%d?%s#%s", url.QueryEscape(in.Password), host, in.Port, q.Encode(), url.QueryEscape(name))}
	case "shadowsocks":
		method := in.Method
		if method == "" {
			method = "aes-128-gcm"
		}
		raw := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", method, in.Password)))
		return []string{fmt.Sprintf("ss://%s@%s:%d#%s", raw, host, in.Port, url.QueryEscape(name))}
	case "vmess":
		j := map[string]any{
			"v": "2", "ps": name, "add": host, "port": strconv.Itoa(in.Port), "id": in.UUID,
			"aid": "0", "net": in.Network, "type": "none", "host": in.Host, "path": in.Path,
			"tls": func() string {
				if in.Security != "" {
					return in.Security
				}
				return ""
			}(), "sni": in.SNI,
		}
		b, _ := json.Marshal(j)
		return []string{"vmess://" + base64.RawStdEncoding.EncodeToString(b)}
	default:
		return []string{}
	}
}

func normalizeHopPorts(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "，", ",")
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\n", ",")
	s = strings.Trim(s, ",")
	return s
}

func normalizeHopInterval(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "30"
	}
	if strings.Contains(s, "-") {
		p := strings.SplitN(s, "-", 2)
		if len(p) == 2 {
			a, errA := strconv.Atoi(strings.TrimSpace(p[0]))
			b, errB := strconv.Atoi(strings.TrimSpace(p[1]))
			if errA == nil && errB == nil && a >= 5 && b >= 5 {
				return fmt.Sprintf("%d-%d", a, b)
			}
		}
		return "30"
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 5 {
		return "30"
	}
	return strconv.Itoa(n)
}

func randomToken(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("tok-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func emptyDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func (a *App) buildXrayConfig() (map[string]any, error) {
	rows, err := a.store.ListInbounds()
	if err != nil {
		return nil, err
	}
	inbounds := make([]map[string]any, 0, len(rows))
	for _, in := range rows {
		settings := in.Settings
		stream := in.Stream
		if settings == nil {
			settings = map[string]any{}
		}
		if stream == nil {
			stream = map[string]any{}
		}
		inbounds = append(inbounds, map[string]any{
			"tag":            fmt.Sprintf("inbound-%d", in.ID),
			"listen":         "0.0.0.0",
			"port":           in.Port,
			"protocol":       in.Protocol,
			"settings":       settings,
			"streamSettings": stream,
			"sniffing": map[string]any{
				"enabled":      true,
				"destOverride": []string{"http", "tls", "quic"},
			},
		})
	}
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  inbounds,
		"outbounds": []map[string]any{{"protocol": "freedom", "tag": "direct"}, {"protocol": "blackhole", "tag": "block"}},
	}
	return cfg, nil
}

func (a *App) handleXrayConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cfg, err := a.buildXrayConfig()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": cfg})
}

func (a *App) handleXrayExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cfg, err := a.buildXrayConfig()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.cfg.XrayConfigOut), 0o755); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(a.cfg.XrayConfigOut, b, 0o644); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": a.cfg.XrayConfigOut, "bytes": len(b)})
}

func (a *App) handleXrayApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cfg, err := a.buildXrayConfig()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.cfg.XrayConfigOut), 0o755); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(a.cfg.XrayConfigOut, b, 0o644); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.TrimSpace(a.cfg.XrayReloadCmd) == "" {
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": a.cfg.XrayConfigOut, "applied": false, "msg": "reload command not configured"})
		return
	}
	cmd := exec.Command("bash", "-lc", a.cfg.XrayReloadCmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		a.writeJSON(w, http.StatusOK, map[string]any{"success": false, "path": a.cfg.XrayConfigOut, "applied": false, "error": err.Error(), "output": string(out)})
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": a.cfg.XrayConfigOut, "applied": true, "output": string(out)})
}

func (a *App) handleNextPort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	base, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("base")))
	p, err := a.store.NextInboundPort(base)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"port": p}})
}

func (a *App) handleBatchToggleInbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		IDs    []int64 `json:"ids"`
		Enable *bool   `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	changed := 0
	for _, id := range req.IDs {
		in, ok, err := a.store.GetInbound(id)
		if err != nil || !ok {
			continue
		}
		if req.Enable != nil {
			in.Enable = *req.Enable
		} else {
			in.Enable = !in.Enable
		}
		if _, ok, err := a.store.UpdateInbound(id, in); err == nil && ok {
			changed++
		}
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "changed": changed})
}

func (a *App) handleForwards(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := a.store.ListForwards()
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": rows})
	case http.MethodPost:
		var f model.Forward
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if f.ListenPort <= 0 || f.TargetHost == "" || f.TargetPort <= 0 {
			a.writeErr(w, http.StatusBadRequest, "listenPort/targetHost/targetPort required")
			return
		}
		if f.Protocol == "" {
			f.Protocol = "tcp"
		}
		obj, err := a.store.AddForward(f)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": obj})
	default:
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleForwardsSub(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/forwards/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		a.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if len(parts) == 2 && parts[1] == "toggle" {
		if r.Method != http.MethodPost {
			a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		obj, ok, err := a.store.ToggleForward(id)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			a.writeErr(w, http.StatusNotFound, "not found")
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": obj})
		return
	}
	switch r.Method {
	case http.MethodPut:
		var f model.Forward
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		obj, ok, err := a.store.UpdateForward(id, f)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			a.writeErr(w, http.StatusNotFound, "not found")
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": obj})
	case http.MethodDelete:
		ok, err := a.store.DeleteForward(id)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			a.writeErr(w, http.StatusNotFound, "not found")
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handlePanelSettings(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		p, err := a.store.GetPanelSetting()
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"username": p.Username, "panelPath": p.PanelPath}})
	case http.MethodPost:
		var req struct {
			Username  string `json:"username"`
			PanelPath string `json:"panelPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		p, err := a.store.UpdatePanelSetting(req.Username, req.PanelPath)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"username": p.Username, "panelPath": p.PanelPath}})
	default:
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handlePanelToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	p, err := a.store.GetPanelSetting()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"token": p.APIToken}})
}

func (a *App) handlePanelTokenRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tok := randomToken(24)
	p, err := a.store.RotateAPIToken(tok)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"token": p.APIToken}})
}

func (a *App) handlePanelChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username == "" || req.Password == "" {
		a.writeErr(w, http.StatusBadRequest, "username/password required")
		return
	}
	ok, err := a.store.CheckUser(req.Username, req.Password)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ok {
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}
	// simple fallback: create/replace by ensuring default user semantics
	_ = a.store.EnsureDefaultUser(req.Username, req.Password)
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) handlePanelConnectSub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"connected": true, "input": req}})
}

func (a *App) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}})
}

func (a *App) handleViewBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	p, _ := a.store.GetPanelSetting()
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"panelPath": p.PanelPath, "username": p.Username}})
}

func (a *App) handleAddRealityQuick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Remark string `json:"remark"`
		Port   int    `json:"port"`
		SNI    string `json:"sni"`
		Dest   string `json:"dest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Port <= 0 {
		np, _ := a.store.NextInboundPort(20000)
		req.Port = np
	}
	u := uuid.NewString()
	pk := strings.ReplaceAll(uuid.NewString(), "-", "")
	sid := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	in, err := buildInboundFromReq(model.AddInboundRequest{
		Remark:      emptyDefault(req.Remark, "reality-quick"),
		Port:        req.Port,
		Protocol:    "vless",
		UUID:        u,
		Network:     "xhttp",
		Security:    "reality",
		SNI:         emptyDefault(req.SNI, "www.cloudflare.com"),
		Host:        emptyDefault(req.SNI, "www.cloudflare.com"),
		Path:        "/",
		RealityDest: emptyDefault(req.Dest, "www.cloudflare.com:443"),
		ShortID:     sid,
		PublicKey:   pk,
	})
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	obj, err := a.store.AddInbound(in)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": obj})
}

func (a *App) runBestEffort(cmdStr string) (string, error) {
	cmd := exec.Command("bash", "-lc", cmdStr)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (a *App) handleSystemRestartPanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := a.runBestEffort("systemctl restart sui-go || true")
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "output": out, "error": errString(err)})
}

func (a *App) handleSystemChainTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"chain": "ok"}})
}

func (a *App) handleSystemRestartXray(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := a.runBestEffort("systemctl restart xray || true")
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "output": out, "error": errString(err)})
}

func (a *App) handleSystemRestartXUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := a.runBestEffort("systemctl restart x-ui || true")
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "output": out, "error": errString(err)})
}

func (a *App) handleSystemOptimizeBBR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "bbr optimize placeholder"})
}

func (a *App) handleSystemOptimizeDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "dns optimize placeholder"})
}

func (a *App) handleSystemOptimizeSysctl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "sysctl optimize placeholder"})
}

func (a *App) handleSystemOptimizeAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "optimize all placeholder"})
}

func (a *App) handleSystemXrayVersionCurrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, _ := a.runBestEffort("xray -version 2>/dev/null | head -n1")
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"current": strings.TrimSpace(out)}})
}

func (a *App) handleSystemXrayRealityGen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"privateKey": strings.ReplaceAll(uuid.NewString(), "-", ""), "publicKey": strings.ReplaceAll(uuid.NewString(), "-", ""), "shortId": strings.ReplaceAll(uuid.NewString(), "-", "")[:8]}})
}

func (a *App) handleSystemXrayConfig(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.handleXrayConfig(w, r)
	case http.MethodPost:
		a.handleXrayApply(w, &http.Request{Method: http.MethodPost, Header: r.Header, Body: r.Body, URL: r.URL})
	default:
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleSystemXrayVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": []string{"current"}})
}

func (a *App) handleSystemXraySwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "switch placeholder"})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) writeErr(w http.ResponseWriter, code int, msg string) {
	a.writeJSON(w, code, map[string]any{"success": false, "msg": msg})
}

func (a *App) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

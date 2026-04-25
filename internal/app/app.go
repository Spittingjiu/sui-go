package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

	reloadMu sync.Mutex
	cacheMu  sync.RWMutex
	cacheKey string
	cacheCfg map[string]any
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
	a.mux.HandleFunc("/api/xray/apply-events", a.handleXrayApplyEvents)
	a.mux.HandleFunc("/api/xray/apply-stats", a.handleXrayApplyStats)

	a.mux.HandleFunc("/api/forwards", a.handleForwards)
	a.mux.HandleFunc("/api/forwards/", a.handleForwardsSub)

	a.mux.HandleFunc("/api/panel/settings", a.handlePanelSettings)
	a.mux.HandleFunc("/api/panel/token", a.handlePanelToken)
	a.mux.HandleFunc("/api/panel/token/rotate", a.handlePanelTokenRotate)
	a.mux.HandleFunc("/api/panel/change-password", a.handlePanelChangePassword)
	a.mux.HandleFunc("/api/panel/connect-sub", a.handlePanelConnectSub)

	a.mux.HandleFunc("/api/system/status", a.handleSystemStatus)
	a.mux.HandleFunc("/api/system/restart-panel", a.handleSystemRestartPanel)
	a.mux.HandleFunc("/api/system/update-panel", a.handleSystemUpdatePanel)
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
	q := r.URL.Query()
	lite := strings.EqualFold(strings.TrimSpace(q.Get("lite")), "1") || strings.EqualFold(strings.TrimSpace(q.Get("lite")), "true")
	if lite {
		limit, _ := strconv.Atoi(strings.TrimSpace(q.Get("limit")))
		offset, _ := strconv.Atoi(strings.TrimSpace(q.Get("offset")))
		if limit < 0 {
			limit = 0
		}
		if limit > 1000 {
			limit = 1000
		}
		if offset < 0 {
			offset = 0
		}
		rows, total, err := a.store.ListInboundsLite(limit, offset)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": rows, "total": total, "limit": limit, "offset": offset, "lite": true})
		return
	}
	full := !strings.EqualFold(strings.TrimSpace(q.Get("full")), "0") && !strings.EqualFold(strings.TrimSpace(q.Get("full")), "false")
	if !full {
		limit, _ := strconv.Atoi(strings.TrimSpace(q.Get("limit")))
		offset, _ := strconv.Atoi(strings.TrimSpace(q.Get("offset")))
		if limit < 0 {
			limit = 0
		}
		if limit > 1000 {
			limit = 1000
		}
		if offset < 0 {
			offset = 0
		}
		rows, total, err := a.store.ListInboundsLite(limit, offset)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": rows, "total": total, "limit": limit, "offset": offset, "lite": true, "full": false})
		return
	}
	rows, err := a.store.ListInbounds()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": rows, "full": true})
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
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, _ := json.Marshal(raw)
	var req model.AddInboundRequest
	if err := json.Unmarshal(b, &req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	mergeXUIStyleIntoAddReq(&req, raw)

	norm, err := a.normalizeAddInboundRequest(req)
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	in, err := buildInboundFromReq(norm)
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
			var raw map[string]any
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				a.writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			b, _ := json.Marshal(raw)
			var req model.AddInboundRequest
			if err := json.Unmarshal(b, &req); err != nil {
				a.writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			mergeXUIStyleIntoAddReq(&req, raw)
			norm, err := a.normalizeAddInboundRequest(req)
			if err != nil {
				a.writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			in, err := buildInboundFromReq(norm)
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
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": buildLinks(in, r)})
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
			links := buildLinks(in, r)
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
				var raw map[string]any
				if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
					a.writeErr(w, http.StatusBadRequest, "invalid json")
					return
				}
				b, _ := json.Marshal(raw)
				var req model.AddInboundRequest
				if err := json.Unmarshal(b, &req); err != nil {
					a.writeErr(w, http.StatusBadRequest, "invalid json")
					return
				}
				mergeXUIStyleIntoAddReq(&req, raw)
				norm, err := a.normalizeAddInboundRequest(req)
				if err != nil {
					a.writeErr(w, http.StatusBadRequest, err.Error())
					return
				}
				in, err := buildInboundFromReq(norm)
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

func (a *App) normalizeAddInboundRequest(req model.AddInboundRequest) (model.AddInboundRequest, error) {
	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	if proto == "" {
		return req, fmt.Errorf("protocol required")
	}
	if req.Port <= 0 {
		np, err := a.store.NextInboundPort(20000)
		if err != nil {
			return req, err
		}
		req.Port = np
	}
	if strings.TrimSpace(req.Remark) == "" {
		req.Remark = fmt.Sprintf("%s-%d", proto, req.Port)
	}
	if req.Enable == nil {
		en := true
		req.Enable = &en
	}
	if req.SniffingEnabled == nil {
		en := true
		req.SniffingEnabled = &en
	}
	if strings.TrimSpace(req.SniffingOverride) == "" {
		req.SniffingOverride = "http,tls,quic"
	}
	if err := validateAddInboundRequest(req); err != nil {
		return req, err
	}
	rows, err := a.store.ListInbounds()
	if err != nil {
		return req, err
	}
	for _, r := range rows {
		if r.Port == req.Port {
			return req, fmt.Errorf("port %d already exists", req.Port)
		}
	}
	if strings.TrimSpace(req.Network) == "" {
		switch proto {
		case "hysteria", "hysteria2":
			req.Network = "hysteria"
		case "vless", "vmess", "trojan", "shadowsocks", "ss":
			req.Network = "tcp"
		}
	}
	switch proto {
	case "vless", "vmess":
		if strings.TrimSpace(req.UUID) == "" {
			req.UUID = uuid.NewString()
		}
	case "trojan", "hysteria", "hysteria2", "shadowsocks", "ss":
		if strings.TrimSpace(req.Password) == "" {
			req.Password = randomToken(8)
		}
	}
	if proto == "shadowsocks" || proto == "ss" {
		if strings.TrimSpace(req.Method) == "" {
			req.Method = "aes-256-gcm"
		}
	}
	if strings.TrimSpace(req.Security) == "" {
		switch proto {
		case "hysteria", "hysteria2", "trojan":
			req.Security = "tls"
		case "vless", "vmess", "shadowsocks", "ss":
			req.Security = "none"
		}
	}
	if strings.EqualFold(req.Security, "tls") || proto == "hysteria" || proto == "hysteria2" || proto == "trojan" {
		if strings.TrimSpace(req.SNI) == "" {
			req.SNI = "www.bing.com"
		}
	}
	if proto == "vless" && strings.EqualFold(req.Security, "reality") {
		if strings.TrimSpace(req.PrivateKey) == "" || strings.TrimSpace(req.PublicKey) == "" {
			out, err := a.runBestEffort("xray x25519 2>/dev/null")
			if err == nil {
				for _, ln := range strings.Split(out, "\n") {
					s := strings.TrimSpace(ln)
					if strings.HasPrefix(s, "PrivateKey:") && strings.TrimSpace(req.PrivateKey) == "" {
						req.PrivateKey = strings.TrimSpace(strings.TrimPrefix(s, "PrivateKey:"))
					}
					if strings.HasPrefix(s, "Password (PublicKey):") && strings.TrimSpace(req.PublicKey) == "" {
						req.PublicKey = strings.TrimSpace(strings.TrimPrefix(s, "Password (PublicKey):"))
					}
				}
			}
		}
		if strings.TrimSpace(req.ShortID) == "" {
			req.ShortID = strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
		}
		if strings.TrimSpace(req.RealityDest) == "" {
			req.RealityDest = "www.cloudflare.com:443"
		}
		if strings.TrimSpace(req.SNI) == "" {
			req.SNI = "www.cloudflare.com"
		}
		if strings.TrimSpace(req.Host) == "" {
			req.Host = req.SNI
		}
	}
	return req, nil
}

func buildInboundFromReq(req model.AddInboundRequest) (model.Inbound, error) {
	if req.Port <= 0 || req.Protocol == "" {
		return model.Inbound{}, fmt.Errorf("port/protocol required")
	}
	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	in := model.Inbound{
		Remark:           req.Remark,
		Port:             req.Port,
		Protocol:         proto,
		Password:         req.Password,
		UUID:             req.UUID,
		Email:            req.Email,
		Method:           req.Method,
		Flow:             req.Flow,
		Network:          req.Network,
		Security:         req.Security,
		SNI:              req.SNI,
		Host:             req.Host,
		Path:             req.Path,
		RealityDest:      req.RealityDest,
		ShortID:          req.ShortID,
		PublicKey:        req.PublicKey,
		PrivateKey:       req.PrivateKey,
		Settings:         map[string]any{},
		Stream:           map[string]any{},
		Extra:            map[string]any{},
		SniffingEnabled:  true,
		SniffingOverride: "http,tls,quic",
	}
	if req.Enable != nil {
		in.Enable = *req.Enable
	}
	if req.SniffingEnabled != nil {
		in.SniffingEnabled = *req.SniffingEnabled
	}
	if strings.TrimSpace(req.SniffingOverride) != "" {
		in.SniffingOverride = req.SniffingOverride
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
		hs := map[string]any{
			"version":        2,
			"auth":           in.Password,
			"udpIdleTimeout": 60,
		}
		if v := strings.TrimSpace(req.HY2Obfs); v != "" {
			hs["obfs"] = v
		}
		if v := strings.TrimSpace(req.HY2ObfsPassword); v != "" {
			hs["obfsPassword"] = v
		}
		if v := strings.TrimSpace(req.HY2Congestion); v != "" {
			hs["congestion"] = v
		}
		if req.HY2UpMbps > 0 {
			hs["up_mbps"] = req.HY2UpMbps
		}
		if req.HY2DownMbps > 0 {
			hs["down_mbps"] = req.HY2DownMbps
		}
		if req.HY2IdleTimeout > 0 {
			hs["udpIdleTimeout"] = req.HY2IdleTimeout
		}
		if req.HY2KeepAlive > 0 {
			hs["keepAlivePeriod"] = req.HY2KeepAlive
		}
		if req.HY2InitStreamRW > 0 {
			hs["initStreamReceiveWindow"] = req.HY2InitStreamRW
		}
		if req.HY2MaxStreamRW > 0 {
			hs["maxStreamReceiveWindow"] = req.HY2MaxStreamRW
		}
		if req.HY2InitConnRW > 0 {
			hs["initConnectionReceiveWindow"] = req.HY2InitConnRW
		}
		if req.HY2MaxConnRW > 0 {
			hs["maxConnectionReceiveWindow"] = req.HY2MaxConnRW
		}
		if req.HY2DisableMTUDisc != nil {
			hs["disablePathMTUDiscovery"] = *req.HY2DisableMTUDisc
		}
		in.Stream = map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"tlsSettings": map[string]any{
				"serverName": emptyDefault(in.SNI, "www.bing.com"),
				"alpn":       []string{"h3"},
				"certificates": []map[string]any{{
					"certificateFile": "/etc/sui-hy2/www.bing.com.crt",
					"keyFile":         "/etc/sui-hy2/www.bing.com.key",
				}},
			},
			"hysteriaSettings": hs,
		}
		hopPorts := normalizeHopPorts(req.HY2HopPorts)
		hopInterval := normalizeHopInterval(req.HY2HopInterval)
		if hopPorts != "" {
			hs := in.Stream["hysteriaSettings"].(map[string]any)
			hs["udphop"] = map[string]any{"ports": hopPorts, "interval": hopInterval}
		}
	case "vless":
		in.Network = normalizeInboundNetwork(in.Network)
		if in.Security == "" {
			in.Security = "none"
		}
		in.Settings = map[string]any{"clients": []map[string]any{{"id": in.UUID, "email": in.Email, "flow": in.Flow}}, "decryption": "none"}
		in.Stream = buildCommonStreamSettings(in.Network, in.Security, in.SNI, in.Host, in.Path)
		if in.Security == "reality" {
			in.Stream["security"] = "reality"
			in.Stream["realitySettings"] = map[string]any{
				"show":        false,
				"dest":        emptyDefault(in.RealityDest, "www.cloudflare.com:443"),
				"serverNames": []string{emptyDefault(in.SNI, "www.cloudflare.com")},
				"privateKey":  in.PrivateKey,
				"shortIds":    []string{in.ShortID},
			}
		}
	case "vmess":
		in.Network = normalizeInboundNetwork(in.Network)
		if in.Security == "" {
			in.Security = "none"
		}
		in.Settings = map[string]any{"clients": []map[string]any{{"id": in.UUID, "alterId": 0, "email": in.Email}}}
		in.Stream = buildCommonStreamSettings(in.Network, in.Security, in.SNI, in.Host, in.Path)
	case "trojan":
		in.Network = normalizeInboundNetwork(in.Network)
		if in.Security == "" {
			in.Security = "tls"
		}
		in.Settings = map[string]any{"clients": []map[string]any{{"password": in.Password, "email": in.Email}}}
		in.Stream = buildCommonStreamSettings(in.Network, in.Security, in.SNI, in.Host, in.Path)
	case "shadowsocks", "ss":
		in.Protocol = "shadowsocks"
		if in.Method == "" {
			in.Method = "aes-128-gcm"
		}
		settings := map[string]any{"method": in.Method, "password": in.Password, "network": "tcp,udp"}
		if req.SSIvCheck != nil {
			settings["ivCheck"] = *req.SSIvCheck
		}
		if len(req.SSClients) > 0 {
			clients := make([]map[string]any, 0, len(req.SSClients))
			is2022 := strings.Contains(strings.ToLower(in.Method), "2022")
			for i, c := range req.SSClients {
				pwd := strings.TrimSpace(c.Password)
				if pwd == "" {
					return model.Inbound{}, fmt.Errorf("ssClients[%d].password required", i)
				}
				client := map[string]any{"password": pwd}
				if !is2022 {
					m := strings.TrimSpace(c.Method)
					if m == "" {
						m = in.Method
					}
					client["method"] = m
				}
				if e := strings.TrimSpace(c.Email); e != "" {
					client["email"] = e
				}
				clients = append(clients, client)
			}
			settings["clients"] = clients
		}
		in.Settings = settings
		in.Network = normalizeInboundNetwork(in.Network)
		if in.Security == "" {
			in.Security = "none"
		}
		in.Stream = buildCommonStreamSettings(in.Network, in.Security, in.SNI, in.Host, in.Path)
	case "socks":
		in.Protocol = "socks"
		authMode := strings.TrimSpace(req.Auth)
		if authMode == "" {
			authMode = "noauth"
		}
		settings := map[string]any{"auth": authMode, "udp": true}
		if authMode == "password" {
			accounts := make([]map[string]any, 0)
			for _, a := range req.SocksAccounts {
				u := strings.TrimSpace(a.User)
				p := a.Pass
				if u == "" || p == "" {
					continue
				}
				accounts = append(accounts, map[string]any{"user": u, "pass": p})
			}
			if len(accounts) == 0 {
				u := strings.TrimSpace(req.AccountUser)
				p := req.AccountPass
				if u == "" || p == "" {
					return model.Inbound{}, fmt.Errorf("socks password auth requires socksAccounts or accountUser/accountPass")
				}
				accounts = append(accounts, map[string]any{"user": u, "pass": p})
			}
			settings["accounts"] = accounts
		}
		in.Settings = settings
		in.Stream = map[string]any{"network": "tcp", "security": "none"}
	case "http":
		in.Protocol = "http"
		accounts := make([]map[string]any, 0)
		for _, a := range req.HTTPAccounts {
			u := strings.TrimSpace(a.User)
			p := a.Pass
			if u == "" || p == "" {
				continue
			}
			accounts = append(accounts, map[string]any{"user": u, "pass": p})
		}
		if len(accounts) == 0 {
			u := strings.TrimSpace(req.AccountUser)
			p := req.AccountPass
			if u != "" && p != "" {
				accounts = append(accounts, map[string]any{"user": u, "pass": p})
			}
		}
		settings := map[string]any{}
		if len(accounts) > 0 {
			settings["accounts"] = accounts
		}
		if req.AllowTransparent != nil {
			settings["allowTransparent"] = *req.AllowTransparent
		}
		in.Settings = settings
		in.Stream = map[string]any{"network": "tcp", "security": "none"}
	case "dokodemo", "dokodemo-door":
		in.Protocol = "dokodemo-door"
		addr := strings.TrimSpace(req.TargetAddress)
		if addr == "" {
			addr = "1.1.1.1"
		}
		port := req.TargetPort
		if port <= 0 {
			port = 53
		}
		in.Settings = map[string]any{
			"address":        addr,
			"port":           port,
			"network":        "tcp,udp",
			"followRedirect": false,
		}
		in.Stream = map[string]any{"network": "tcp", "security": "none"}
	case "wireguard":
		in.Protocol = "wireguard"
		mtu := req.WireguardMTU
		if mtu <= 0 {
			mtu = 1420
		}
		sk := strings.TrimSpace(req.WireguardSecretKey)
		if sk == "" {
			sk, _ = generateWireguardKeypair()
		}
		peers := make([]map[string]any, 0)
		if len(req.WireguardPeers) > 0 {
			for i, p := range req.WireguardPeers {
				pub := strings.TrimSpace(p.PublicKey)
				if pub == "" {
					return model.Inbound{}, fmt.Errorf("wireguardPeers[%d].publicKey required", i)
				}
				allowed := make([]string, 0)
				for _, ip := range p.AllowedIPs {
					v := strings.TrimSpace(ip)
					if v == "" {
						continue
					}
					if !strings.Contains(v, "/") {
						v += "/32"
					}
					allowed = append(allowed, v)
				}
				if len(allowed) == 0 {
					allowed = []string{"10.0.0.2/32"}
				}
				peer := map[string]any{
					"publicKey":  pub,
					"allowedIPs": allowed,
				}
				if pk := strings.TrimSpace(p.PrivateKey); pk != "" {
					peer["privateKey"] = pk
				}
				if psk := strings.TrimSpace(p.PreSharedKey); psk != "" {
					peer["preSharedKey"] = psk
				}
				if p.KeepAlive > 0 {
					peer["keepAlive"] = p.KeepAlive
				}
				peers = append(peers, peer)
			}
		}
		if len(peers) == 0 {
			peerAllowed := strings.TrimSpace(req.WireguardAddress)
			if peerAllowed == "" {
				peerAllowed = "10.0.0.2/32"
			}
			if !strings.Contains(peerAllowed, "/") {
				peerAllowed += "/32"
			}
			_, pk := generateWireguardKeypair()
			peers = append(peers, map[string]any{
				"publicKey":  pk,
				"allowedIPs": []string{peerAllowed},
				"keepAlive":  0,
			})
		}
		noKernelTun := false
		if req.WireguardNoKernelTun != nil {
			noKernelTun = *req.WireguardNoKernelTun
		}
		in.Settings = map[string]any{
			"mtu":         mtu,
			"secretKey":   sk,
			"peers":       peers,
			"noKernelTun": noKernelTun,
		}
		in.Stream = map[string]any{"network": "tcp", "security": "none"}
	case "tun":
		in.Protocol = "tun"
		name := strings.TrimSpace(req.TunName)
		if name == "" {
			name = "xray0"
		}
		mtu := req.TunMTU
		if mtu <= 0 {
			mtu = 1500
		}
		stack := strings.TrimSpace(req.TunStack)
		if stack == "" {
			stack = "system"
		}
		userLevel := req.TunUserLevel
		if userLevel < 0 {
			userLevel = 0
		}
		in.Settings = map[string]any{
			"name":        name,
			"mtu":         mtu,
			"stack":       stack,
			"autoRoute":   req.TunAutoRoute,
			"strictRoute": req.TunStrictRoute,
			"userLevel":   userLevel,
		}
		in.Stream = map[string]any{"network": "tcp", "security": "none"}
	default:
		return model.Inbound{}, fmt.Errorf("unsupported protocol: %s", proto)
	}
	applyAdvancedTransportAndTLS(req, &in)
	if len(req.SettingsOverride) > 0 {
		in.Settings = mergeAnyMap(in.Settings, req.SettingsOverride)
	}
	if len(req.StreamOverride) > 0 {
		in.Stream = mergeAnyMap(in.Stream, req.StreamOverride)
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

func isLoopbackOrLocalHost(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" || h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "[::1]" {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

func stripHostPort(raw string) string {
	h := strings.TrimSpace(raw)
	if h == "" {
		return ""
	}
	if strings.Contains(h, ",") {
		h = strings.TrimSpace(strings.SplitN(h, ",", 2)[0])
	}
	if strings.HasPrefix(h, "[") && strings.Contains(h, "]") {
		if host, _, err := net.SplitHostPort(h); err == nil {
			return strings.Trim(host, "[]")
		}
		return strings.Trim(h, "[]")
	}
	if strings.Count(h, ":") == 1 {
		if host, _, err := net.SplitHostPort(h); err == nil {
			return host
		}
	}
	return h
}

func detectLocalIPv4() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok && ua.IP != nil {
		ip := ua.IP.String()
		if strings.Count(ip, ":") == 0 {
			return ip
		}
	}
	return ""
}

func resolveLinkHost(r *http.Request) string {
	if env := strings.TrimSpace(os.Getenv("SUI_PUBLIC_HOST")); env != "" {
		return stripHostPort(env)
	}
	candidates := []string{
		r.Header.Get("X-Forwarded-Host"),
		r.Header.Get("X-Original-Host"),
		r.Host,
	}
	for _, c := range candidates {
		h := stripHostPort(c)
		if h != "" && !isLoopbackOrLocalHost(h) {
			return h
		}
	}
	if ip := detectLocalIPv4(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

func buildLinks(in model.Inbound, r *http.Request) []string {
	host := resolveLinkHost(r)
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
		query.Set("insecure", "1")
		portPart := strconv.Itoa(in.Port)
		if hs, ok := in.Stream["hysteriaSettings"].(map[string]any); ok {
			if obfs, ok := hs["obfs"].(string); ok && strings.TrimSpace(obfs) != "" {
				query.Set("obfs", strings.TrimSpace(obfs))
			}
			if obfsPwd, ok := hs["obfsPassword"].(string); ok && strings.TrimSpace(obfsPwd) != "" {
				query.Set("obfs-password", strings.TrimSpace(obfsPwd))
			}
			if hop, ok := hs["udphop"].(map[string]any); ok {
				if ports, ok := hop["ports"].(string); ok && strings.TrimSpace(ports) != "" {
					// 与 x-ui 兼容：端口跳跃写入 mport，不再放 authority 多端口
					query.Set("mport", strings.TrimSpace(ports))
				}
				if iv, ok := hop["interval"].(string); ok && strings.TrimSpace(iv) != "" {
					ivs := strings.TrimSpace(iv)
					if strings.Contains(ivs, "-") {
						p := strings.SplitN(ivs, "-", 2)
						if len(p) == 2 {
							a := strings.TrimSpace(p[0])
							b := strings.TrimSpace(p[1])
							if _, errA := strconv.Atoi(a); errA == nil {
								if _, errB := strconv.Atoi(b); errB == nil {
									query.Set("udphopIntervalMin", a)
									query.Set("udphopIntervalMax", b)
								}
							}
						}
					} else if _, err := strconv.Atoi(ivs); err == nil {
						query.Set("udphopInterval", ivs)
					}
				}
			}
		}
		h := host
		if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
			h = "[" + h + "]"
		}
		return []string{fmt.Sprintf("hy2://%s@%s:%s?%s#%s", url.QueryEscape(in.Password), h, portPart, query.Encode(), url.QueryEscape(name))}
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

func normalizeInboundNetwork(raw string) string {
	n := strings.TrimSpace(strings.ToLower(raw))
	switch n {
	case "", "raw":
		return "tcp"
	case "tcp", "kcp", "ws", "grpc", "httpupgrade", "xhttp", "hysteria":
		return n
	default:
		return "tcp"
	}
}

func buildCommonStreamSettings(network, security, sni, host, path string) map[string]any {
	network = normalizeInboundNetwork(network)
	sec := strings.TrimSpace(strings.ToLower(security))
	if sec == "" {
		sec = "none"
	}
	stream := map[string]any{"network": network, "security": sec}
	safePath := emptyDefault(path, "/")
	safeHost := emptyDefault(host, sni)
	switch network {
	case "ws":
		stream["wsSettings"] = map[string]any{"path": safePath, "headers": map[string]any{"Host": safeHost}}
	case "xhttp":
		stream["xhttpSettings"] = map[string]any{"path": safePath, "host": safeHost, "mode": "auto"}
	case "httpupgrade":
		stream["httpupgradeSettings"] = map[string]any{"path": safePath, "host": safeHost}
	case "grpc":
		stream["grpcSettings"] = map[string]any{"serviceName": strings.TrimPrefix(safePath, "/")}
	}
	if sec == "tls" {
		stream["tlsSettings"] = map[string]any{"serverName": emptyDefault(sni, host)}
	}
	return stream
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func applyAdvancedTransportAndTLS(req model.AddInboundRequest, in *model.Inbound) {
	if in == nil || in.Stream == nil {
		return
	}

	// TLS 细项（对齐 x-ui 常见字段）
	if tls, ok := in.Stream["tlsSettings"].(map[string]any); ok {
		if alpn := parseCSV(req.TLSALPN); len(alpn) > 0 {
			tls["alpn"] = alpn
		}
		settings := map[string]any{}
		if v := strings.TrimSpace(req.TLSFingerprint); v != "" {
			settings["fingerprint"] = v
			tls["fingerprint"] = v
		}
		if req.TLSAllowInsecure != nil {
			settings["allowInsecure"] = *req.TLSAllowInsecure
			tls["allowInsecure"] = *req.TLSAllowInsecure
		}
		if v := strings.TrimSpace(req.TLSMinVersion); v != "" {
			tls["minVersion"] = v
		}
		if v := strings.TrimSpace(req.TLSMaxVersion); v != "" {
			tls["maxVersion"] = v
		}
		if cs := parseCSV(req.TLSCipherSuites); len(cs) > 0 {
			tls["cipherSuites"] = cs
		}
		if len(settings) > 0 {
			tls["settings"] = settings
		}
	}

	netw := strings.ToLower(strings.TrimSpace(in.Network))
	if netw == "" {
		netw = strings.ToLower(strings.TrimSpace(req.Network))
	}

	// KCP 细项
	if netw == "kcp" {
		k := map[string]any{}
		if req.KCPMtu > 0 {
			k["mtu"] = req.KCPMtu
		}
		if req.KCPTti > 0 {
			k["tti"] = req.KCPTti
		}
		if req.KCPUplinkCapacity > 0 {
			k["uplinkCapacity"] = req.KCPUplinkCapacity
		}
		if req.KCPDownlinkCapacity > 0 {
			k["downlinkCapacity"] = req.KCPDownlinkCapacity
		}
		if req.KCPCongestion != nil {
			k["congestion"] = *req.KCPCongestion
		}
		if req.KCPReadBufferSize > 0 {
			k["readBufferSize"] = req.KCPReadBufferSize
		}
		if req.KCPWriteBufferSize > 0 {
			k["writeBufferSize"] = req.KCPWriteBufferSize
		}
		headType := strings.TrimSpace(req.KCPHeaderType)
		seed := strings.TrimSpace(req.KCPSeed)
		if headType != "" || seed != "" {
			h := map[string]any{}
			if headType != "" {
				h["type"] = headType
			}
			if seed != "" {
				h["request"] = map[string]any{"seed": seed}
			}
			k["header"] = h
		}
		if len(k) > 0 {
			in.Stream["kcpSettings"] = mergeAnyMap(map[string]any{}, k)
		}
	}

	// gRPC 细项
	if netw == "grpc" {
		g := map[string]any{}
		if v := strings.TrimSpace(req.GrpcServiceName); v != "" {
			g["serviceName"] = strings.TrimPrefix(v, "/")
		}
		if v := strings.TrimSpace(req.GrpcAuthority); v != "" {
			g["authority"] = v
		}
		if req.GrpcMultiMode != nil {
			g["multiMode"] = *req.GrpcMultiMode
		}
		if len(g) > 0 {
			in.Stream["grpcSettings"] = mergeAnyMap(map[string]any{}, g)
		}
	}

	// xhttp 细项
	if netw == "xhttp" {
		x := map[string]any{}
		if v := strings.TrimSpace(req.XHTTPMode); v != "" {
			x["mode"] = v
		}
		if v := strings.TrimSpace(req.XHTTPHost); v != "" {
			x["host"] = v
		}
		if v := strings.TrimSpace(req.XHTTPPath); v != "" {
			x["path"] = v
		}
		if len(x) > 0 {
			in.Stream["xhttpSettings"] = mergeAnyMap(map[string]any{}, x)
		}
	}
}

var (
	reUUID     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	reShortID  = regexp.MustCompile(`^[0-9a-fA-F]{1,16}$`)
	reHostLike = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

var allowedOverrideSettingsKeys = map[string]map[string]struct{}{
	"hysteria": {
		"version": {}, "clients": {},
	},
	"vless": {
		"clients": {}, "decryption": {}, "fallbacks": {},
	},
	"vmess": {
		"clients": {}, "disableInsecureEncryption": {},
	},
	"trojan": {
		"clients": {}, "fallbacks": {},
	},
	"shadowsocks": {
		"method": {}, "password": {}, "network": {}, "clients": {}, "ivCheck": {},
	},
	"socks": {
		"auth": {}, "accounts": {}, "udp": {}, "ip": {},
	},
	"http": {
		"accounts": {}, "allowTransparent": {}, "timeout": {},
	},
	"dokodemo-door": {
		"address": {}, "port": {}, "portMap": {}, "network": {}, "followRedirect": {},
	},
	"wireguard": {
		"mtu": {}, "secretKey": {}, "peers": {}, "noKernelTun": {}, "reserved": {},
	},
	"tun": {
		"name": {}, "mtu": {}, "stack": {}, "autoRoute": {}, "strictRoute": {}, "userLevel": {},
	},
}

var allowedOverrideStreamKeys = map[string]struct{}{
	"network": {}, "security": {}, "tlsSettings": {}, "realitySettings": {},
	"grpcSettings": {}, "wsSettings": {}, "kcpSettings": {}, "xhttpSettings": {},
	"httpupgradeSettings": {}, "hysteriaSettings": {}, "sockopt": {}, "quicSettings": {},
}

func hasUnknownKeys(patch map[string]any, allow map[string]struct{}) []string {
	if len(patch) == 0 {
		return nil
	}
	bad := make([]string, 0)
	for k := range patch {
		if _, ok := allow[k]; !ok {
			bad = append(bad, k)
		}
	}
	return bad
}

func validateAddInboundRequest(req model.AddInboundRequest) error {
	if req.Port <= 0 || req.Port > 65535 {
		return fmt.Errorf("invalid port")
	}
	switch req.Port {
	case 22, 25, 53, 80, 443, 3306, 6379:
		return fmt.Errorf("port %d is reserved", req.Port)
	}

	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	if (proto == "vless" || proto == "vmess") && strings.TrimSpace(req.UUID) != "" && !reUUID.MatchString(strings.TrimSpace(req.UUID)) {
		return fmt.Errorf("invalid uuid format")
	}
	if proto == "vless" && strings.EqualFold(strings.TrimSpace(req.Security), "reality") {
		sid := strings.TrimSpace(req.ShortID)
		if sid != "" && !reShortID.MatchString(sid) {
			return fmt.Errorf("invalid shortId format")
		}
	}
	if sni := strings.TrimSpace(req.SNI); sni != "" && !reHostLike.MatchString(sni) {
		return fmt.Errorf("invalid sni format")
	}
	if p := strings.TrimSpace(req.Path); p != "" && !strings.HasPrefix(p, "/") {
		return fmt.Errorf("path must start with /")
	}

	canonProto := proto
	if canonProto == "ss" {
		canonProto = "shadowsocks"
	}
	if canonProto == "dokodemo" {
		canonProto = "dokodemo-door"
	}
	if len(req.SettingsOverride) > 0 {
		allow := allowedOverrideSettingsKeys[canonProto]
		if len(allow) == 0 {
			return fmt.Errorf("settingsOverride not allowed for protocol %s", proto)
		}
		if bad := hasUnknownKeys(req.SettingsOverride, allow); len(bad) > 0 {
			return fmt.Errorf("settingsOverride has unknown keys: %s", strings.Join(bad, ","))
		}
	}
	if len(req.StreamOverride) > 0 {
		if bad := hasUnknownKeys(req.StreamOverride, allowedOverrideStreamKeys); len(bad) > 0 {
			return fmt.Errorf("streamOverride has unknown keys: %s", strings.Join(bad, ","))
		}
	}

	switch proto {
	case "shadowsocks", "ss":
		if m := strings.TrimSpace(req.Method); m != "" {
			if strings.Contains(strings.ToLower(m), "2022") && strings.TrimSpace(req.Password) == "" && len(req.SSClients) == 0 {
				return fmt.Errorf("ss 2022 method requires password or ssClients")
			}
		}
		for i, c := range req.SSClients {
			if strings.TrimSpace(c.Password) == "" {
				return fmt.Errorf("ssClients[%d].password required", i)
			}
		}
	case "socks":
		authMode := strings.ToLower(strings.TrimSpace(req.Auth))
		if authMode == "password" {
			ok := len(req.SocksAccounts) > 0 || (strings.TrimSpace(req.AccountUser) != "" && req.AccountPass != "")
			if !ok {
				return fmt.Errorf("socks password auth requires socksAccounts or accountUser/accountPass")
			}
		}
	case "http":
		if len(req.HTTPAccounts) > 0 {
			for i, a := range req.HTTPAccounts {
				if strings.TrimSpace(a.User) == "" || strings.TrimSpace(a.Pass) == "" {
					return fmt.Errorf("httpAccounts[%d] user/pass required", i)
				}
			}
		}
	case "wireguard":
		if req.WireguardMTU > 0 && (req.WireguardMTU < 1280 || req.WireguardMTU > 9000) {
			return fmt.Errorf("wireguard mtu out of range")
		}
		for i, p := range req.WireguardPeers {
			if strings.TrimSpace(p.PublicKey) == "" {
				return fmt.Errorf("wireguardPeers[%d].publicKey required", i)
			}
			if p.KeepAlive < 0 || p.KeepAlive > 600 {
				return fmt.Errorf("wireguardPeers[%d].keepAlive out of range", i)
			}
		}
	case "tun":
		if req.TunMTU > 0 && (req.TunMTU < 1280 || req.TunMTU > 9000) {
			return fmt.Errorf("tun mtu out of range")
		}
	}
	return nil
}

func asMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func asSliceMap(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func toStr(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

func toBoolPtr(v any) *bool {
	switch b := v.(type) {
	case bool:
		x := b
		return &x
	case string:
		s := strings.ToLower(strings.TrimSpace(b))
		if s == "true" || s == "1" {
			x := true
			return &x
		}
		if s == "false" || s == "0" {
			x := false
			return &x
		}
	}
	return nil
}

func mergeXUIStyleIntoAddReq(req *model.AddInboundRequest, raw map[string]any) {
	if req == nil || raw == nil {
		return
	}
	if v := toStr(raw["listen"]); v != "" {
		// 占位兼容：x-ui 有 listen 字段，当前后端统一监听 0.0.0.0，不单独落库
	}
	if req.Security == "" {
		if stream := asMap(raw["streamSettings"]); stream != nil {
			if v := toStr(stream["security"]); v != "" {
				req.Security = v
			}
		}
	}
	if stream := asMap(raw["streamSettings"]); stream != nil {
		if req.Network == "" {
			if v := toStr(stream["network"]); v != "" {
				req.Network = v
			}
		}
		if tls := asMap(stream["tlsSettings"]); tls != nil {
			if req.SNI == "" {
				req.SNI = toStr(tls["serverName"])
			}
			if req.TLSALPN == "" {
				if arr, ok := tls["alpn"].([]any); ok && len(arr) > 0 {
					parts := make([]string, 0, len(arr))
					for _, a := range arr {
						if s := toStr(a); s != "" {
							parts = append(parts, s)
						}
					}
					req.TLSALPN = strings.Join(parts, ",")
				}
			}
			if req.TLSFingerprint == "" {
				req.TLSFingerprint = toStr(tls["fingerprint"])
			}
			if req.TLSAllowInsecure == nil {
				req.TLSAllowInsecure = toBoolPtr(tls["allowInsecure"])
			}
		}
		if ws := asMap(stream["wsSettings"]); ws != nil {
			if req.Path == "" {
				req.Path = toStr(ws["path"])
			}
			if req.Host == "" {
				if h := asMap(ws["headers"]); h != nil {
					req.Host = toStr(h["Host"])
				}
			}
		}
		if grpc := asMap(stream["grpcSettings"]); grpc != nil {
			if req.GrpcServiceName == "" {
				req.GrpcServiceName = toStr(grpc["serviceName"])
			}
			if req.GrpcAuthority == "" {
				req.GrpcAuthority = toStr(grpc["authority"])
			}
			if req.GrpcMultiMode == nil {
				req.GrpcMultiMode = toBoolPtr(grpc["multiMode"])
			}
		}
	}
	settings := asMap(raw["settings"])
	if settings == nil {
		return
	}
	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	switch proto {
	case "vless", "vmess":
		if req.UUID == "" {
			if cs := asSliceMap(settings["clients"]); len(cs) > 0 {
				req.UUID = toStr(cs[0]["id"])
				req.Email = toStr(cs[0]["email"])
				req.Flow = toStr(cs[0]["flow"])
			}
		}
	case "trojan":
		if req.Password == "" {
			if cs := asSliceMap(settings["clients"]); len(cs) > 0 {
				req.Password = toStr(cs[0]["password"])
				req.Email = toStr(cs[0]["email"])
			}
		}
	case "shadowsocks", "ss":
		if req.Method == "" {
			req.Method = toStr(settings["method"])
		}
		if req.Password == "" {
			req.Password = toStr(settings["password"])
		}
		if req.SSIvCheck == nil {
			req.SSIvCheck = toBoolPtr(settings["ivCheck"])
		}
		if len(req.SSClients) == 0 {
			if cs := asSliceMap(settings["clients"]); len(cs) > 0 {
				out := make([]model.SSClientInput, 0, len(cs))
				for _, c := range cs {
					out = append(out, model.SSClientInput{Method: toStr(c["method"]), Password: toStr(c["password"]), Email: toStr(c["email"])})
				}
				req.SSClients = out
			}
		}
	case "socks":
		if req.Auth == "" {
			req.Auth = toStr(settings["auth"])
		}
		if len(req.SocksAccounts) == 0 {
			if as := asSliceMap(settings["accounts"]); len(as) > 0 {
				out := make([]model.UserPassInput, 0, len(as))
				for _, a := range as {
					out = append(out, model.UserPassInput{User: toStr(a["user"]), Pass: toStr(a["pass"])})
				}
				req.SocksAccounts = out
			}
		}
	case "http":
		if req.AllowTransparent == nil {
			req.AllowTransparent = toBoolPtr(settings["allowTransparent"])
		}
		if len(req.HTTPAccounts) == 0 {
			if as := asSliceMap(settings["accounts"]); len(as) > 0 {
				out := make([]model.UserPassInput, 0, len(as))
				for _, a := range as {
					out = append(out, model.UserPassInput{User: toStr(a["user"]), Pass: toStr(a["pass"])})
				}
				req.HTTPAccounts = out
			}
		}
	case "wireguard":
		if req.WireguardMTU == 0 {
			req.WireguardMTU = toInt(settings["mtu"])
		}
		if req.WireguardSecretKey == "" {
			req.WireguardSecretKey = toStr(settings["secretKey"])
		}
		if req.WireguardNoKernelTun == nil {
			req.WireguardNoKernelTun = toBoolPtr(settings["noKernelTun"])
		}
		if len(req.WireguardPeers) == 0 {
			if ps := asSliceMap(settings["peers"]); len(ps) > 0 {
				out := make([]model.WireguardPeerInput, 0, len(ps))
				for _, p := range ps {
					ips := []string{}
					if arr, ok := p["allowedIPs"].([]any); ok {
						for _, a := range arr {
							if s := toStr(a); s != "" {
								ips = append(ips, s)
							}
						}
					}
					out = append(out, model.WireguardPeerInput{PublicKey: toStr(p["publicKey"]), PrivateKey: toStr(p["privateKey"]), PreSharedKey: toStr(p["preSharedKey"]), AllowedIPs: ips, KeepAlive: toInt(p["keepAlive"])})
				}
				req.WireguardPeers = out
			}
		}
	case "tun":
		if req.TunName == "" {
			req.TunName = toStr(settings["name"])
		}
		if req.TunMTU == 0 {
			req.TunMTU = toInt(settings["mtu"])
		}
		if req.TunStack == "" {
			req.TunStack = toStr(settings["stack"])
		}
		if req.TunUserLevel == 0 {
			req.TunUserLevel = toInt(settings["userLevel"])
		}
	}
}

func mergeAnyMap(base map[string]any, patch map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for k, v := range patch {
		if vm, ok := v.(map[string]any); ok {
			if bm, ok2 := base[k].(map[string]any); ok2 {
				base[k] = mergeAnyMap(bm, vm)
			} else {
				base[k] = mergeAnyMap(map[string]any{}, vm)
			}
			continue
		}
		base[k] = v
	}
	return base
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

func generateWireguardKeypair() (privateKey, publicKey string) {
	out, err := exec.Command("bash", "-lc", "xray wg 2>/dev/null").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, ln := range lines {
			s := strings.TrimSpace(ln)
			if strings.HasPrefix(s, "PrivateKey:") {
				privateKey = strings.TrimSpace(strings.TrimPrefix(s, "PrivateKey:"))
			}
			if strings.HasPrefix(s, "Password (PublicKey):") {
				publicKey = strings.TrimSpace(strings.TrimPrefix(s, "Password (PublicKey):"))
			}
		}
	}
	if privateKey == "" {
		privateKey = base64.StdEncoding.EncodeToString([]byte(randomToken(24)))
	}
	if publicKey == "" {
		publicKey = base64.StdEncoding.EncodeToString([]byte(randomToken(24)))
	}
	return privateKey, publicKey
}

func emptyDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func computeInboundCacheKey(rows []model.Inbound) string {
	h := sha256.New()
	for _, in := range rows {
		_, _ = h.Write([]byte(fmt.Sprintf("%d|%s|%d|%s|%t|%s|", in.ID, in.Remark, in.Port, in.Protocol, in.Enable, in.SniffingOverride)))
		sb, _ := json.Marshal(in.Settings)
		stb, _ := json.Marshal(in.Stream)
		_, _ = h.Write(sb)
		_, _ = h.Write(stb)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	b, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return in
	}
	return out
}

func (a *App) buildXrayConfig() (map[string]any, error) {
	rows, err := a.store.ListInbounds()
	if err != nil {
		return nil, err
	}
	key := computeInboundCacheKey(rows)
	a.cacheMu.RLock()
	if a.cacheCfg != nil && a.cacheKey == key {
		cfg := cloneAnyMap(a.cacheCfg)
		a.cacheMu.RUnlock()
		return cfg, nil
	}
	a.cacheMu.RUnlock()

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
		destOverride := []string{"http", "tls", "quic"}
		if strings.TrimSpace(in.SniffingOverride) != "" {
			parts := strings.Split(in.SniffingOverride, ",")
			tmp := make([]string, 0, len(parts))
			for _, p := range parts {
				v := strings.TrimSpace(p)
				if v != "" {
					tmp = append(tmp, v)
				}
			}
			if len(tmp) > 0 {
				destOverride = tmp
			}
		}
		inbounds = append(inbounds, map[string]any{
			"tag":            fmt.Sprintf("inbound-%d", in.ID),
			"listen":         "0.0.0.0",
			"port":           in.Port,
			"protocol":       in.Protocol,
			"settings":       settings,
			"streamSettings": stream,
			"sniffing": map[string]any{
				"enabled":      in.SniffingEnabled,
				"destOverride": destOverride,
			},
		})
	}
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  inbounds,
		"outbounds": []map[string]any{{"protocol": "freedom", "tag": "direct"}, {"protocol": "blackhole", "tag": "block"}},
	}
	a.cacheMu.Lock()
	a.cacheKey = key
	a.cacheCfg = cloneAnyMap(cfg)
	a.cacheMu.Unlock()
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

func (a *App) appendApplyEvent(ev map[string]any) {
	if ev == nil {
		return
	}
	if err := os.MkdirAll("data", 0o755); err != nil {
		return
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	f, err := os.OpenFile("data/apply-events.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line)
	_, _ = f.Write([]byte("\n"))
}

func (a *App) handleXrayApplyEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	b, err := os.ReadFile("data/apply-events.jsonl")
	if err != nil {
		if os.IsNotExist(err) {
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": []any{}, "limit": limit})
			return
		}
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": []any{}, "limit": limit})
		return
	}
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	out := make([]map[string]any, 0, len(lines)-start)
	for _, ln := range lines[start:] {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(ln), &obj); err == nil {
			out = append(out, obj)
		}
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": out, "limit": limit})
}

func (a *App) handleXrayApplyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sinceMin, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("sinceMin")))
	if sinceMin <= 0 {
		sinceMin = 60
	}
	if sinceMin > 1440 {
		sinceMin = 1440
	}
	b, err := os.ReadFile("data/apply-events.jsonl")
	if err != nil {
		if os.IsNotExist(err) {
			a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"sinceMin": sinceMin, "total": 0, "ok": 0, "fail": 0, "skipped": 0, "rolledBack": 0, "successRate": 0}})
			return
		}
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cut := time.Now().Add(-time.Duration(sinceMin) * time.Minute).Unix()
	total, okCnt, failCnt, skipped, rolledBack := 0, 0, 0, 0, 0
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(ln), &obj); err != nil {
			continue
		}
		ts := int64(0)
		switch v := obj["ts"].(type) {
		case float64:
			ts = int64(v)
		case int64:
			ts = v
		}
		if ts < cut {
			continue
		}
		total++
		if b, _ := obj["success"].(bool); b {
			okCnt++
		} else {
			failCnt++
		}
		if b, _ := obj["skipped"].(bool); b {
			skipped++
		}
		if b, _ := obj["rolledBack"].(bool); b {
			rolledBack++
		}
	}
	rate := 0.0
	if total > 0 {
		rate = float64(okCnt) * 100 / float64(total)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"sinceMin": sinceMin, "total": total, "ok": okCnt, "fail": failCnt, "skipped": skipped, "rolledBack": rolledBack, "successRate": rate}})
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
	now := time.Now().Unix()
	existing, _ := os.ReadFile(a.cfg.XrayConfigOut)
	if bytes.Equal(existing, b) {
		a.appendApplyEvent(map[string]any{"ts": now, "success": true, "applied": false, "skipped": true, "reason": "unchanged"})
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": a.cfg.XrayConfigOut, "applied": false, "skipped": true, "msg": "config unchanged, skip reload"})
		return
	}
	if err := os.WriteFile(a.cfg.XrayConfigOut, b, 0o644); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.TrimSpace(a.cfg.XrayReloadCmd) == "" {
		a.appendApplyEvent(map[string]any{"ts": now, "success": true, "applied": false, "skipped": true, "reason": "reload_not_configured"})
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": a.cfg.XrayConfigOut, "applied": false, "msg": "reload command not configured"})
		return
	}
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()

	backupPath := a.cfg.XrayConfigOut + ".last-good"
	if len(existing) > 0 {
		_ = os.WriteFile(backupPath, existing, 0o644)
	}

	reload := func() (string, error, bool) {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-lc", a.cfg.XrayReloadCmd)
		out, err := cmd.CombinedOutput()
		if ctx.Err() == context.DeadlineExceeded {
			return string(out), fmt.Errorf("reload timeout"), true
		}
		return string(out), err, false
	}

	out, err, timeout := reload()
	if err != nil {
		rolled := false
		rollbackErr := ""
		if len(existing) > 0 {
			if werr := os.WriteFile(a.cfg.XrayConfigOut, existing, 0o644); werr == nil {
				_, rerr, _ := reload()
				if rerr == nil {
					rolled = true
				} else {
					rollbackErr = rerr.Error()
				}
			} else {
				rollbackErr = werr.Error()
			}
		}
		a.appendApplyEvent(map[string]any{"ts": now, "success": false, "applied": false, "rolledBack": rolled, "rollbackError": rollbackErr, "error": err.Error(), "timeout": timeout})
		a.writeJSON(w, http.StatusOK, map[string]any{"success": false, "path": a.cfg.XrayConfigOut, "applied": false, "rolledBack": rolled, "rollbackError": rollbackErr, "error": err.Error(), "output": out})
		return
	}
	_ = os.WriteFile(backupPath, b, 0o644)
	a.appendApplyEvent(map[string]any{"ts": now, "success": true, "applied": true, "backup": backupPath})
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": a.cfg.XrayConfigOut, "applied": true, "output": out, "backup": backupPath})
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
	maskToken := func(tok string) string {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			return ""
		}
		if len(tok) <= 12 {
			return tok
		}
		return tok[:6] + "..." + tok[len(tok)-6:]
	}
	switch r.Method {
	case http.MethodGet:
		p, err := a.store.GetPanelSetting()
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{
			"username":           p.Username,
			"panelPath":          p.PanelPath,
			"forceResetPassword": false,
			"panelTokenMasked":   maskToken(p.APIToken),
		}})
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
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{
			"username":           p.Username,
			"panelPath":          p.PanelPath,
			"forceResetPassword": false,
			"panelTokenMasked":   maskToken(p.APIToken),
		}})
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
	_, user, ok := a.extractAuth(r)
	if !ok {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(strings.TrimSpace(req.NewPassword)) < 6 {
		a.writeErr(w, http.StatusBadRequest, "new password too short")
		return
	}
	changed, err := a.store.ChangeUserPassword(user, req.OldPassword, req.NewPassword)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !changed {
		a.writeErr(w, http.StatusBadRequest, "old password incorrect")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "password updated"})
}

func inferPanelBaseURL(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
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
	var req struct {
		SubURL      string `json:"subUrl"`
		SubUsername string `json:"subUsername"`
		SubPassword string `json:"subPassword"`
		SourceName  string `json:"sourceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.SubURL = strings.TrimSpace(req.SubURL)
	req.SubUsername = strings.TrimSpace(req.SubUsername)
	req.SourceName = strings.TrimSpace(req.SourceName)
	if req.SourceName == "" {
		req.SourceName = "sui-go"
	}
	if req.SubURL == "" || req.SubUsername == "" || req.SubPassword == "" {
		a.writeErr(w, http.StatusBadRequest, "subUrl / subUsername / subPassword 必填")
		return
	}

	p, err := a.store.GetPanelSetting()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
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

	base := strings.TrimRight(req.SubURL, "/")
	client := &http.Client{Timeout: 12 * time.Second}

	loginBody, _ := json.Marshal(map[string]string{"username": req.SubUsername, "password": req.SubPassword})
	loginReq, _ := http.NewRequest(http.MethodPost, base+"/api/auth/login", strings.NewReader(string(loginBody)))
	loginReq.Header.Set("content-type", "application/json")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		a.writeErr(w, http.StatusBadGateway, "连接 sui-sub 失败: "+err.Error())
		return
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode < 200 || loginResp.StatusCode >= 300 {
		a.writeErr(w, http.StatusBadRequest, fmt.Sprintf("sui-sub 登录失败 HTTP %d", loginResp.StatusCode))
		return
	}
	cookies := loginResp.Cookies()
	if len(cookies) == 0 {
		a.writeErr(w, http.StatusBadRequest, "sui-sub 登录未返回会话 cookie")
		return
	}
	cookieHeader := make([]string, 0, len(cookies))
	for _, c := range cookies {
		cookieHeader = append(cookieHeader, c.Name+"="+c.Value)
	}

	panelBase := inferPanelBaseURL(r)
	sourceBody, _ := json.Marshal(map[string]string{
		"name":        req.SourceName,
		"panel_url":   panelBase,
		"panel_token": panelToken,
	})
	sourceReq, _ := http.NewRequest(http.MethodPost, base+"/api/sources", strings.NewReader(string(sourceBody)))
	sourceReq.Header.Set("content-type", "application/json")
	sourceReq.Header.Set("cookie", strings.Join(cookieHeader, "; "))
	sourceResp, err := client.Do(sourceReq)
	if err != nil {
		a.writeErr(w, http.StatusBadGateway, "写入 sui-sub 失败: "+err.Error())
		return
	}
	defer sourceResp.Body.Close()
	var sourceReply map[string]any
	_ = json.NewDecoder(sourceResp.Body).Decode(&sourceReply)
	if sourceResp.StatusCode < 200 || sourceResp.StatusCode >= 300 || sourceReply["ok"] != true {
		msg := fmt.Sprintf("sui-sub 对接失败 HTTP %d", sourceResp.StatusCode)
		if em, ok := sourceReply["error"].(string); ok && strings.TrimSpace(em) != "" {
			msg = em
		}
		a.writeErr(w, http.StatusBadRequest, msg)
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"msg":     "已写入到 sui-sub",
		"obj": map[string]any{
			"panelUrl":   panelBase,
			"sourceName": req.SourceName,
		},
	})
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
	panelSvc := map[string]any{"name": "sui-go", "active": "unknown", "enabled": "unknown"}
	xraySvc := map[string]any{"name": "xray", "active": "unknown", "enabled": "unknown"}
	if out, err := a.runBestEffort("systemctl is-active sui-go 2>/dev/null || true"); err == nil {
		v := strings.TrimSpace(out)
		if v == "" {
			v = "inactive"
		}
		panelSvc["active"] = v
	}
	if out, err := a.runBestEffort("systemctl is-enabled sui-go 2>/dev/null || true"); err == nil {
		v := strings.TrimSpace(out)
		if v == "" {
			v = "disabled"
		}
		panelSvc["enabled"] = v
	}
	if out, err := a.runBestEffort("systemctl is-active xray 2>/dev/null || true"); err == nil {
		v := strings.TrimSpace(out)
		if v == "" {
			v = "inactive"
		}
		xraySvc["active"] = v
	}
	if out, err := a.runBestEffort("systemctl is-enabled xray 2>/dev/null || true"); err == nil {
		v := strings.TrimSpace(out)
		if v == "" {
			v = "disabled"
		}
		xraySvc["enabled"] = v
	}
	rows, _ := a.store.ListInbounds()
	enabled := 0
	for _, in := range rows {
		if in.Enable {
			enabled++
		}
	}
	xver := map[string]any{"binary": "", "panel": "self-hosted"}
	if out, err := a.runBestEffort("xray version 2>/dev/null | head -n1"); err == nil {
		xver["binary"] = strings.TrimSpace(out)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{
		"panel": map[string]any{"username": p.Username, "panelPath": p.PanelPath, "forceResetPassword": false},
		"status": map[string]any{
			"panel":           panelSvc,
			"xray":            xraySvc,
			"xrayVersion":     xver["binary"],
			"inboundsTotal":   len(rows),
			"inboundsEnabled": enabled,
		},
		"xrayVersion": xver,
	}})
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
	prv := strings.ReplaceAll(uuid.NewString(), "-", "")
	sid := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	inReq, err := a.normalizeAddInboundRequest(model.AddInboundRequest{
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
		PrivateKey:  prv,
	})
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	in, err := buildInboundFromReq(inReq)
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

func (a *App) handleSystemUpdatePanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	panelDir := os.Getenv("PANEL_APP_DIR")
	if strings.TrimSpace(panelDir) == "" {
		panelDir = "/opt/sui-go"
	}
	cmd := fmt.Sprintf("set -e; cd %s; test -d .git; branch=$(git rev-parse --abbrev-ref HEAD || echo main); git fetch --all --prune; git pull --ff-only origin \"$branch\"; go build -o sui-go ./cmd/sui-go; install -m 0755 sui-go /usr/local/bin/sui-go; (nohup systemctl restart sui-go >/dev/null 2>&1 &) ; echo updated:$branch", shellQuote(panelDir))
	out, err := a.runBestEffort(cmd)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, strings.TrimSpace(out)+"\n"+err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "panel updated", "output": strings.TrimSpace(out), "dir": panelDir})
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
	obj, err := a.applyBbrFq()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": obj, "msg": "BBR 已应用"})
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
	obj, err := a.applyDNSProfile()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": obj, "msg": "DNS 配置已应用"})
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
	obj, err := a.applyNetSysctlProfile()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": obj, "msg": "网络栈参数已应用"})
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
	bbr, err := a.applyBbrFq()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dns, err := a.applyDNSProfile()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sysctlObj, err := a.applyNetSysctlProfile()
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"bbr": bbr, "dns": dns, "sysctl": sysctlObj}, "msg": "全部优化已应用"})
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
	out, _ := a.runBestEffort("xray version 2>/dev/null | head -n1")
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"binary": strings.TrimSpace(out), "panel": "self-hosted"}})
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
	out, err := a.runBestEffort("xray x25519 2>/dev/null")
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pri, pub := "", ""
	for _, ln := range strings.Split(out, "\n") {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, "PrivateKey:") {
			pri = strings.TrimSpace(strings.TrimPrefix(s, "PrivateKey:"))
		}
		if strings.HasPrefix(s, "Password (PublicKey):") {
			pub = strings.TrimSpace(strings.TrimPrefix(s, "Password (PublicKey):"))
		}
	}
	if pri == "" || pub == "" {
		a.writeErr(w, http.StatusInternalServerError, "failed to parse x25519 output")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"privateKey": pri, "publicKey": pub, "shortId": strings.ReplaceAll(uuid.NewString(), "-", "")[:8], "spiderX": "/"}})
}

func (a *App) handleSystemXrayConfig(w http.ResponseWriter, r *http.Request) {
	if !a.checkAuth(r) {
		a.writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		b, err := os.ReadFile(a.cfg.XrayConfigOut)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": map[string]any{"path": a.cfg.XrayConfigOut, "content": string(b)}})
	case http.MethodPost:
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(req.Content) == "" {
			a.writeErr(w, http.StatusBadRequest, "配置内容不能为空")
			return
		}
		var parsed any
		if err := json.Unmarshal([]byte(req.Content), &parsed); err != nil {
			a.writeErr(w, http.StatusBadRequest, "JSON 格式错误")
			return
		}
		tmp := fmt.Sprintf("%s.tmp-%d.json", a.cfg.XrayConfigOut, time.Now().UnixMilli())
		pretty, _ := json.MarshalIndent(parsed, "", "  ")
		if err := os.WriteFile(tmp, pretty, 0o644); err != nil {
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if out, err := a.runBestEffort(fmt.Sprintf("xray run -test -config %s", shellQuote(tmp))); err != nil {
			_ = os.Remove(tmp)
			a.writeErr(w, http.StatusBadRequest, strings.TrimSpace(out))
			return
		}
		if err := os.Rename(tmp, a.cfg.XrayConfigOut); err != nil {
			_ = os.Remove(tmp)
			a.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "配置已保存并通过校验（未自动重启）"})
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
	out, err := a.runBestEffort("curl -fsSL https://api.github.com/repos/XTLS/Xray-core/releases?per_page=20")
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, strings.TrimSpace(out))
		return
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	versions := make([]string, 0, len(arr))
	for _, it := range arr {
		if tag, ok := it["tag_name"].(string); ok && strings.TrimSpace(tag) != "" {
			versions = append(versions, strings.TrimSpace(tag))
		}
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": versions})
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
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ver := strings.TrimSpace(req.Version)
	if ver == "" {
		a.writeErr(w, http.StatusBadRequest, "version required")
		return
	}
	cmd := strings.Join([]string{
		"set -e",
		"TMP=$(mktemp -d)",
		fmt.Sprintf("curl -fL --retry 3 -o \"$TMP/xray.zip\" \"https://github.com/XTLS/Xray-core/releases/download/%s/Xray-linux-64.zip\"", ver),
		"unzip -o \"$TMP/xray.zip\" -d \"$TMP\" >/dev/null",
		"install -m 0755 \"$TMP/xray\" /usr/local/bin/xray",
		"[ -f \"$TMP/geoip.dat\" ] && install -m 0644 \"$TMP/geoip.dat\" /usr/local/share/xray-geoip.dat || true",
		"[ -f \"$TMP/geosite.dat\" ] && install -m 0644 \"$TMP/geosite.dat\" /usr/local/share/xray-geosite.dat || true",
		"rm -rf \"$TMP\"",
		"systemctl daemon-reload || true",
		"systemctl restart xray || true",
		"/usr/local/bin/xray version 2>/dev/null | head -n1",
	}, " && ")
	out, err := a.runBestEffort(cmd)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, strings.TrimSpace(out))
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "msg": "switched to " + ver, "current": strings.TrimSpace(out)})
}

func (a *App) applyBbrFq() (map[string]any, error) {
	conf := strings.Join([]string{
		"net.core.default_qdisc=fq",
		"net.ipv4.tcp_congestion_control=bbr",
	}, "\n") + "\n"
	if err := os.WriteFile("/etc/sysctl.d/99-sui-bbr.conf", []byte(conf), 0o644); err != nil {
		return nil, err
	}
	_, _ = a.runBestEffort("modprobe tcp_bbr || true")
	if out, err := a.runBestEffort("sysctl --system >/dev/null"); err != nil {
		return nil, fmt.Errorf(strings.TrimSpace(out))
	}
	qdisc, _ := a.runBestEffort("sysctl -n net.core.default_qdisc || true")
	cc, _ := a.runBestEffort("sysctl -n net.ipv4.tcp_congestion_control || true")
	return map[string]any{"qdisc": strings.TrimSpace(qdisc), "cc": strings.TrimSpace(cc)}, nil
}

func (a *App) applyNetSysctlProfile() (map[string]any, error) {
	conf := strings.Join([]string{
		"fs.file-max = 1048576",
		"net.core.somaxconn = 65535",
		"net.core.netdev_max_backlog = 32768",
		"net.ipv4.tcp_max_syn_backlog = 8192",
		"net.ipv4.ip_local_port_range = 1024 65535",
		"net.ipv4.tcp_fin_timeout = 15",
		"net.ipv4.tcp_tw_reuse = 1",
		"net.core.rmem_max = 67108864",
		"net.core.wmem_max = 67108864",
		"net.ipv4.tcp_rmem = 4096 87380 33554432",
		"net.ipv4.tcp_wmem = 4096 65536 33554432",
		"net.ipv4.tcp_mtu_probing = 1",
	}, "\n") + "\n"
	if err := os.WriteFile("/etc/sysctl.d/99-sui-net.conf", []byte(conf), 0o644); err != nil {
		return nil, err
	}
	if out, err := a.runBestEffort("sysctl --system >/dev/null"); err != nil {
		return nil, fmt.Errorf(strings.TrimSpace(out))
	}
	return map[string]any{"ok": true}, nil
}

func (a *App) applyDNSProfile() (map[string]any, error) {
	dir := "/etc/systemd/resolved.conf.d"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	conf := strings.Join([]string{
		"[Resolve]",
		"DNS=1.1.1.1 8.8.8.8 2606:4700:4700::1111 2001:4860:4860::8888",
		"FallbackDNS=9.9.9.9 1.0.0.1 2620:fe::fe 2606:4700:4700::1001",
		"DNSStubListener=yes",
		"DNSSEC=no",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "99-sui-dns.conf"), []byte(conf), 0o644); err != nil {
		return nil, err
	}
	_, _ = a.runBestEffort("systemctl restart systemd-resolved || true")
	return map[string]any{"ok": true}, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
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
	if code == http.StatusOK {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(code)
	buf := bytes.NewBuffer(make([]byte, 0, 512))
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		_, _ = w.Write([]byte(`{"success":false,"msg":"encode response failed"}\n`))
		return
	}
	_, _ = w.Write(buf.Bytes())
}

package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Spittingjiu/sui-go/internal/model"
	"github.com/Spittingjiu/sui-go/internal/store"
)

type Config struct {
	Addr      string
	DataFile  string
	DBFile    string
	PanelUser string
	PanelPass string
}

type App struct {
	cfg   Config
	mux   *http.ServeMux
	store *store.SQLiteStore
}

func New(cfg Config) (*App, error) {
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
	a.mux.HandleFunc("/api/inbounds", a.handleListInbounds)
	a.mux.HandleFunc("/api/inbounds/add", a.handleAddInbound)
	a.mux.HandleFunc("/api/inbounds/", a.handleInboundSub)

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
		in.Settings = map[string]any{"clients": []map[string]any{{"id": in.UUID, "email": in.Email, "flow": in.Flow}}, "decryption": "none"}
		in.Stream = map[string]any{"network": in.Network, "security": in.Security}
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
		if in.Security == "reality" {
			if in.PublicKey != "" {
				q.Set("pbk", in.PublicKey)
			}
			if in.ShortID != "" {
				q.Set("sid", in.ShortID)
			}
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

func (a *App) writeErr(w http.ResponseWriter, code int, msg string) {
	a.writeJSON(w, code, map[string]any{"success": false, "msg": msg})
}

func (a *App) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

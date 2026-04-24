package app

import (
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
	Addr     string
	DataFile string
}

type App struct {
	cfg   Config
	mux   *http.ServeMux
	store *store.Store
}

func New(cfg Config) (*App, error) {
	st, err := store.New(cfg.DataFile)
	if err != nil {
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
	a.mux.HandleFunc("/api/inbounds", a.handleListInbounds)
	a.mux.HandleFunc("/api/inbounds/add", a.handleAddInbound)
	a.mux.HandleFunc("/api/inbounds/", a.handleInboundSub)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": "sui-go"})
}

func (a *App) handleListInbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": a.store.List()})
}

func (a *App) handleAddInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req model.AddInboundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Port <= 0 || req.Protocol == "" {
		a.writeErr(w, http.StatusBadRequest, "port/protocol required")
		return
	}

	in := model.Inbound{
		Remark:   req.Remark,
		Port:     req.Port,
		Protocol: req.Protocol,
		Password: req.Password,
		UUID:     req.UUID,
		Email:    req.Email,
		Network:  req.Network,
		Security: req.Security,
		SNI:      req.SNI,
		Settings: map[string]any{},
		Stream:   map[string]any{},
		Extra:    map[string]any{},
	}

	if strings.EqualFold(req.Protocol, "hysteria") || strings.EqualFold(req.Protocol, "hysteria2") {
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
			hs["udphop"] = map[string]any{
				"ports":    hopPorts,
				"interval": hopInterval,
			}
		}
	}

	created, err := a.store.Add(in)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": created})
}

func (a *App) handleInboundSub(w http.ResponseWriter, r *http.Request) {
	// /api/inbounds/:id/links
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/inbounds/"), "/")
	if len(parts) != 2 || parts[1] != "links" {
		a.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	in, ok := a.store.Get(id)
	if !ok {
		a.writeErr(w, http.StatusNotFound, "not found")
		return
	}
	links := buildLinks(in)
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": links})
}

func buildLinks(in model.Inbound) []string {
	if in.Protocol != "hysteria" {
		return []string{}
	}
	host := "127.0.0.1"
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
	name := in.Remark
	if name == "" {
		name = fmt.Sprintf("inbound-%d", time.Now().Unix())
	}
	return []string{fmt.Sprintf("hy2://%s@%s:%d?%s#%s", url.QueryEscape(in.Password), host, in.Port, query.Encode(), url.QueryEscape(name))}
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
	// allow N or A-B; fallback 30
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

func (a *App) writeErr(w http.ResponseWriter, code int, msg string) {
	a.writeJSON(w, code, map[string]any{"success": false, "msg": msg})
}

func (a *App) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

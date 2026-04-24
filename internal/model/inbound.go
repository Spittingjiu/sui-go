package model

type Inbound struct {
	ID         int64          `json:"id"`
	Remark     string         `json:"remark"`
	Port       int            `json:"port"`
	Protocol   string         `json:"protocol"`
	Password   string         `json:"password,omitempty"`
	UUID       string         `json:"uuid,omitempty"`
	Email      string         `json:"email,omitempty"`
	Network    string         `json:"network,omitempty"`
	Security   string         `json:"security,omitempty"`
	SNI        string         `json:"sni,omitempty"`
	Enable     bool           `json:"enable"`
	Settings   map[string]any `json:"settings,omitempty"`
	Stream     map[string]any `json:"streamSettings,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
	CreateUnix int64          `json:"createUnix"`
	UpdateUnix int64          `json:"updateUnix"`
}

type AddInboundRequest struct {
	Remark         string `json:"remark"`
	Port           int    `json:"port"`
	Protocol       string `json:"protocol"`
	Password       string `json:"password"`
	UUID           string `json:"uuid"`
	Email          string `json:"email"`
	Network        string `json:"network"`
	Security       string `json:"security"`
	SNI            string `json:"sni"`
	HY2HopPorts    string `json:"hy2HopPorts"`
	HY2HopInterval string `json:"hy2HopInterval"`
}

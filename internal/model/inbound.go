package model

type Inbound struct {
	ID          int64          `json:"id"`
	Remark      string         `json:"remark"`
	Port        int            `json:"port"`
	Protocol    string         `json:"protocol"`
	Password    string         `json:"password,omitempty"`
	UUID        string         `json:"uuid,omitempty"`
	Email       string         `json:"email,omitempty"`
	Method      string         `json:"method,omitempty"`
	Flow        string         `json:"flow,omitempty"`
	Network     string         `json:"network,omitempty"`
	Security    string         `json:"security,omitempty"`
	SNI         string         `json:"sni,omitempty"`
	Host        string         `json:"host,omitempty"`
	Path        string         `json:"path,omitempty"`
	RealityDest string         `json:"realityDest,omitempty"`
	ShortID     string         `json:"shortId,omitempty"`
	PublicKey   string         `json:"publicKey,omitempty"`
	Enable      bool           `json:"enable"`
	Settings    map[string]any `json:"settings,omitempty"`
	Stream      map[string]any `json:"streamSettings,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
	CreateUnix  int64          `json:"createUnix"`
	UpdateUnix  int64          `json:"updateUnix"`
}

type AddInboundRequest struct {
	Remark         string `json:"remark"`
	Port           int    `json:"port"`
	Protocol       string `json:"protocol"`
	Password       string `json:"password"`
	UUID           string `json:"uuid"`
	Email          string `json:"email"`
	Method         string `json:"method"`
	Flow           string `json:"flow"`
	Network        string `json:"network"`
	Security       string `json:"security"`
	SNI            string `json:"sni"`
	Host           string `json:"host"`
	Path           string `json:"path"`
	RealityDest    string `json:"realityDest"`
	ShortID        string `json:"shortId"`
	PublicKey      string `json:"publicKey"`
	HY2HopPorts    string `json:"hy2HopPorts"`
	HY2HopInterval string `json:"hy2HopInterval"`
}

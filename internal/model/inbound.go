package model

type Inbound struct {
	ID               int64          `json:"id"`
	Remark           string         `json:"remark"`
	Port             int            `json:"port"`
	Protocol         string         `json:"protocol"`
	Password         string         `json:"password,omitempty"`
	UUID             string         `json:"uuid,omitempty"`
	Email            string         `json:"email,omitempty"`
	Method           string         `json:"method,omitempty"`
	Flow             string         `json:"flow,omitempty"`
	Network          string         `json:"network,omitempty"`
	Security         string         `json:"security,omitempty"`
	SNI              string         `json:"sni,omitempty"`
	Host             string         `json:"host,omitempty"`
	Path             string         `json:"path,omitempty"`
	RealityDest      string         `json:"realityDest,omitempty"`
	ShortID          string         `json:"shortId,omitempty"`
	PublicKey        string         `json:"publicKey,omitempty"`
	PrivateKey       string         `json:"privateKey,omitempty"`
	Enable           bool           `json:"enable"`
	Settings         map[string]any `json:"settings,omitempty"`
	Stream           map[string]any `json:"streamSettings,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
	SniffingEnabled  bool           `json:"sniffingEnabled"`
	SniffingOverride string         `json:"sniffingDestOverride,omitempty"`
	CreateUnix       int64          `json:"createUnix"`
	UpdateUnix       int64          `json:"updateUnix"`
}

type UserPassInput struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

type SSClientInput struct {
	Method   string `json:"method"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

type WireguardPeerInput struct {
	PublicKey    string   `json:"publicKey"`
	PrivateKey   string   `json:"privateKey"`
	PreSharedKey string   `json:"preSharedKey"`
	AllowedIPs   []string `json:"allowedIPs"`
	KeepAlive    int      `json:"keepAlive"`
}

type AddInboundRequest struct {
	Remark               string               `json:"remark"`
	Port                 int                  `json:"port"`
	Protocol             string               `json:"protocol"`
	Password             string               `json:"password"`
	UUID                 string               `json:"uuid"`
	Email                string               `json:"email"`
	Method               string               `json:"method"`
	Flow                 string               `json:"flow"`
	Network              string               `json:"network"`
	Security             string               `json:"security"`
	SNI                  string               `json:"sni"`
	Host                 string               `json:"host"`
	Path                 string               `json:"path"`
	RealityDest          string               `json:"realityDest"`
	ShortID              string               `json:"shortId"`
	PublicKey            string               `json:"publicKey"`
	PrivateKey           string               `json:"privateKey"`
	HY2HopPorts          string               `json:"hy2HopPorts"`
	HY2HopInterval       string               `json:"hy2HopInterval"`
	TLSALPN              string               `json:"tlsAlpn"`
	TLSFingerprint       string               `json:"tlsFingerprint"`
	TLSAllowInsecure     *bool                `json:"tlsAllowInsecure"`
	TLSMinVersion        string               `json:"tlsMinVersion"`
	TLSMaxVersion        string               `json:"tlsMaxVersion"`
	TLSCipherSuites      string               `json:"tlsCipherSuites"`
	KCPMtu               int                  `json:"kcpMtu"`
	KCPTti               int                  `json:"kcpTti"`
	KCPUplinkCapacity    int                  `json:"kcpUplinkCapacity"`
	KCPDownlinkCapacity  int                  `json:"kcpDownlinkCapacity"`
	KCPCongestion        *bool                `json:"kcpCongestion"`
	KCPReadBufferSize    int                  `json:"kcpReadBufferSize"`
	KCPWriteBufferSize   int                  `json:"kcpWriteBufferSize"`
	KCPSeed              string               `json:"kcpSeed"`
	KCPHeaderType        string               `json:"kcpHeaderType"`
	GrpcServiceName      string               `json:"grpcServiceName"`
	GrpcAuthority        string               `json:"grpcAuthority"`
	GrpcMultiMode        *bool                `json:"grpcMultiMode"`
	XHTTPMode            string               `json:"xhttpMode"`
	XHTTPHost            string               `json:"xhttpHost"`
	XHTTPPath            string               `json:"xhttpPath"`
	TargetAddress        string               `json:"targetAddress"`
	TargetPort           int                  `json:"targetPort"`
	Auth                 string               `json:"auth"`
	AccountUser          string               `json:"accountUser"`
	AccountPass          string               `json:"accountPass"`
	SocksAccounts        []UserPassInput      `json:"socksAccounts"`
	HTTPAccounts         []UserPassInput      `json:"httpAccounts"`
	AllowTransparent     *bool                `json:"allowTransparent"`
	SSClients            []SSClientInput      `json:"ssClients"`
	SSIvCheck            *bool                `json:"ssIvCheck"`
	WireguardSecretKey   string               `json:"wireguardSecretKey"`
	WireguardAddress     string               `json:"wireguardAddress"`
	WireguardMTU         int                  `json:"wireguardMtu"`
	WireguardReserved    string               `json:"wireguardReserved"`
	WireguardPeers       []WireguardPeerInput `json:"wireguardPeers"`
	WireguardNoKernelTun *bool                `json:"wireguardNoKernelTun"`
	TunName              string               `json:"tunName"`
	TunMTU               int                  `json:"tunMtu"`
	TunStack             string               `json:"tunStack"`
	TunAutoRoute         bool                 `json:"tunAutoRoute"`
	TunStrictRoute       bool                 `json:"tunStrictRoute"`
	TunUserLevel         int                  `json:"tunUserLevel"`
	SniffingEnabled      *bool                `json:"sniffingEnabled"`
	SniffingOverride     string               `json:"sniffingDestOverride"`
	Enable               *bool                `json:"enable"`
	ExpiryTime           int64                `json:"expiryTime"`
	HY2Obfs              string               `json:"hy2Obfs"`
	HY2ObfsPassword      string               `json:"hy2ObfsPassword"`
	HY2Congestion        string               `json:"hy2Congestion"`
	HY2UpMbps            int                  `json:"hy2UpMbps"`
	HY2DownMbps          int                  `json:"hy2DownMbps"`
	HY2IdleTimeout       int                  `json:"hy2IdleTimeout"`
	HY2KeepAlive         int                  `json:"hy2KeepAlivePeriod"`
	HY2InitStreamRW      int                  `json:"hy2InitStreamReceiveWindow"`
	HY2MaxStreamRW       int                  `json:"hy2MaxStreamReceiveWindow"`
	HY2InitConnRW        int                  `json:"hy2InitConnectionReceiveWindow"`
	HY2MaxConnRW         int                  `json:"hy2MaxConnectionReceiveWindow"`
	HY2DisableMTUDisc    *bool                `json:"hy2DisableMtuDiscovery"`
	SettingsOverride     map[string]any       `json:"settingsOverride"`
	StreamOverride       map[string]any       `json:"streamOverride"`
}

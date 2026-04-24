package model

type Forward struct {
	ID         int64  `json:"id"`
	Remark     string `json:"remark"`
	ListenPort int    `json:"listenPort"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
	Protocol   string `json:"protocol"` // tcp/udp/both
	Enable     bool   `json:"enable"`
}

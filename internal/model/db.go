package model

import "time"

type InboundDB struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Remark           string    `json:"remark"`
	Port             int       `json:"port"`
	Protocol         string    `json:"protocol"`
	Password         string    `json:"password"`
	UUID             string    `json:"uuid"`
	Email            string    `json:"email"`
	Method           string    `json:"method"`
	Flow             string    `json:"flow"`
	Network          string    `json:"network"`
	Security         string    `json:"security"`
	SNI              string    `json:"sni"`
	Host             string    `json:"host"`
	Path             string    `json:"path"`
	RealityDest      string    `json:"realityDest"`
	ShortID          string    `json:"shortId"`
	PublicKey        string    `json:"publicKey"`
	PrivateKey       string    `json:"privateKey"`
	Enable           bool      `json:"enable"`
	Settings         string    `json:"settings"`
	Stream           string    `json:"streamSettings"`
	Tag              string    `json:"tag"`
	SniffingEnabled  bool      `json:"sniffingEnabled" gorm:"default:true"`
	SniffingOverride string    `json:"sniffingDestOverride" gorm:"default:http,tls,quic"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type UserDB struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex" json:"username"`
	Password     string    `json:"-"`
	LastLoginAt  time.Time `gorm:"index" json:"lastLoginAt"`
	FailedLogins int       `gorm:"default:0" json:"failedLogins"`
	LockedUntil  time.Time `gorm:"index" json:"lockedUntil"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type TokenDB struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Token     string    `gorm:"uniqueIndex;size:96" json:"token"`
	Username  string    `gorm:"index;size:64" json:"username"`
	ExpiresAt time.Time `gorm:"index" json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ForwardDB struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Remark     string    `json:"remark"`
	ListenPort int       `json:"listenPort"`
	TargetHost string    `json:"targetHost"`
	TargetPort int       `json:"targetPort"`
	Protocol   string    `json:"protocol"`
	Enable     bool      `json:"enable"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type PanelSettingDB struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `json:"username"`
	PanelPath string    `json:"panelPath"`
	APIToken  string    `json:"apiToken"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

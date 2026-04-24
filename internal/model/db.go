package model

import "time"

type InboundDB struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Remark    string    `json:"remark"`
	Port      int       `json:"port"`
	Protocol  string    `json:"protocol"`
	Password  string    `json:"password"`
	Network   string    `json:"network"`
	Security  string    `json:"security"`
	SNI       string    `json:"sni"`
	Enable    bool      `json:"enable"`
	Settings  string    `json:"settings"`
	Stream    string    `json:"streamSettings"`
	Tag       string    `json:"tag"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UserDB struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex" json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type TokenDB struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Token     string    `gorm:"uniqueIndex;size:96" json:"token"`
	Username  string    `gorm:"index;size:64" json:"username"`
	ExpiresAt time.Time `gorm:"index" json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

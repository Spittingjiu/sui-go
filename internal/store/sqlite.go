package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Spittingjiu/sui-go/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type SQLiteStore struct {
	db *gorm.DB
}

func NewSQLite(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.InboundDB{}, &model.UserDB{}); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) EnsureDefaultUser(username, password string) error {
	var cnt int64
	if err := s.db.Model(&model.UserDB{}).Where("username = ?", username).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	u := model.UserDB{Username: username, Password: password}
	return s.db.Create(&u).Error
}

func (s *SQLiteStore) CheckUser(username, password string) (bool, error) {
	var u model.UserDB
	err := s.db.Where("username = ?", username).First(&u).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return u.Password == password, nil
}

func (s *SQLiteStore) ListInbounds() ([]model.Inbound, error) {
	var rows []model.InboundDB
	if err := s.db.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.Inbound, 0, len(rows))
	for _, r := range rows {
		var settings map[string]any
		var stream map[string]any
		_ = json.Unmarshal([]byte(r.Settings), &settings)
		_ = json.Unmarshal([]byte(r.Stream), &stream)
		out = append(out, model.Inbound{
			ID:         int64(r.ID),
			Remark:     r.Remark,
			Port:       r.Port,
			Protocol:   r.Protocol,
			Password:   r.Password,
			Network:    r.Network,
			Security:   r.Security,
			SNI:        r.SNI,
			Enable:     r.Enable,
			Settings:   settings,
			Stream:     stream,
			CreateUnix: r.CreatedAt.Unix(),
			UpdateUnix: r.UpdatedAt.Unix(),
		})
	}
	return out, nil
}

func (s *SQLiteStore) GetInbound(id int64) (model.Inbound, bool, error) {
	var r model.InboundDB
	if err := s.db.First(&r, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.Inbound{}, false, nil
		}
		return model.Inbound{}, false, err
	}
	var settings map[string]any
	var stream map[string]any
	_ = json.Unmarshal([]byte(r.Settings), &settings)
	_ = json.Unmarshal([]byte(r.Stream), &stream)
	return model.Inbound{
		ID:         int64(r.ID),
		Remark:     r.Remark,
		Port:       r.Port,
		Protocol:   r.Protocol,
		Password:   r.Password,
		Network:    r.Network,
		Security:   r.Security,
		SNI:        r.SNI,
		Enable:     r.Enable,
		Settings:   settings,
		Stream:     stream,
		CreateUnix: r.CreatedAt.Unix(),
		UpdateUnix: r.UpdatedAt.Unix(),
	}, true, nil
}

func (s *SQLiteStore) AddInbound(in model.Inbound) (model.Inbound, error) {
	settings, _ := json.Marshal(in.Settings)
	stream, _ := json.Marshal(in.Stream)
	row := model.InboundDB{
		Remark:   in.Remark,
		Port:     in.Port,
		Protocol: in.Protocol,
		Password: in.Password,
		Network:  in.Network,
		Security: in.Security,
		SNI:      in.SNI,
		Enable:   true,
		Settings: string(settings),
		Stream:   string(stream),
		Tag:      fmt.Sprintf("inbound-%d", time.Now().UnixNano()),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return model.Inbound{}, err
	}
	in.ID = int64(row.ID)
	in.Enable = true
	in.CreateUnix = row.CreatedAt.Unix()
	in.UpdateUnix = row.UpdatedAt.Unix()
	return in, nil
}

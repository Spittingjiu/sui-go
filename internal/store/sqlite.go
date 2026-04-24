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
	if err := db.AutoMigrate(&model.InboundDB{}, &model.UserDB{}, &model.TokenDB{}); err != nil {
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

func (s *SQLiteStore) UpdateInbound(id int64, in model.Inbound) (model.Inbound, bool, error) {
	var row model.InboundDB
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.Inbound{}, false, nil
		}
		return model.Inbound{}, false, err
	}
	settings, _ := json.Marshal(in.Settings)
	stream, _ := json.Marshal(in.Stream)
	row.Remark = in.Remark
	row.Port = in.Port
	row.Protocol = in.Protocol
	row.Password = in.Password
	row.Network = in.Network
	row.Security = in.Security
	row.SNI = in.SNI
	row.Settings = string(settings)
	row.Stream = string(stream)
	if err := s.db.Save(&row).Error; err != nil {
		return model.Inbound{}, false, err
	}
	got, ok, err := s.GetInbound(id)
	return got, ok, err
}

func (s *SQLiteStore) DeleteInbound(id int64) (bool, error) {
	res := s.db.Delete(&model.InboundDB{}, id)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (s *SQLiteStore) SaveToken(token, username string, expiresAt time.Time) error {
	t := model.TokenDB{Token: token, Username: username, ExpiresAt: expiresAt}
	return s.db.Create(&t).Error
}

func (s *SQLiteStore) ValidateToken(token string, now time.Time) (string, bool, error) {
	var t model.TokenDB
	err := s.db.Where("token = ?", token).First(&t).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	if !t.ExpiresAt.After(now) {
		_ = s.db.Delete(&t).Error
		return "", false, nil
	}
	return t.Username, true, nil
}

func (s *SQLiteStore) RefreshToken(oldToken, newToken string, expiresAt time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.TokenDB{}).Where("token = ?", oldToken).Updates(map[string]any{
			"token":      newToken,
			"expires_at": expiresAt,
		}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *SQLiteStore) DeleteToken(token string) error {
	return s.db.Where("token = ?", token).Delete(&model.TokenDB{}).Error
}

func (s *SQLiteStore) CleanupExpiredTokens(now time.Time) error {
	return s.db.Where("expires_at <= ?", now).Delete(&model.TokenDB{}).Error
}

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
	if err := db.AutoMigrate(&model.InboundDB{}, &model.UserDB{}, &model.TokenDB{}, &model.ForwardDB{}, &model.PanelSettingDB{}); err != nil {
		return nil, err
	}
	_ = db.Exec("ALTER TABLE inbound_dbs ADD COLUMN sniffing_enabled numeric DEFAULT 1").Error
	_ = db.Exec("ALTER TABLE inbound_dbs ADD COLUMN sniffing_override text DEFAULT 'http,tls,quic'").Error
	_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_inbound_dbs_port ON inbound_dbs(port)").Error
	_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_inbound_dbs_protocol ON inbound_dbs(protocol)").Error
	_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_inbound_dbs_enable ON inbound_dbs(enable)").Error
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) EnsureDefaultPanelSetting(username string) error {
	var cnt int64
	if err := s.db.Model(&model.PanelSettingDB{}).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	p := model.PanelSettingDB{Username: username, PanelPath: "/", APIToken: ""}
	return s.db.Create(&p).Error
}

func (s *SQLiteStore) GetPanelSetting() (model.PanelSettingDB, error) {
	var p model.PanelSettingDB
	err := s.db.First(&p).Error
	return p, err
}

func (s *SQLiteStore) UpdatePanelSetting(username, panelPath string) (model.PanelSettingDB, error) {
	p, err := s.GetPanelSetting()
	if err != nil {
		return model.PanelSettingDB{}, err
	}
	if username != "" {
		p.Username = username
	}
	if panelPath != "" {
		p.PanelPath = panelPath
	}
	if err := s.db.Save(&p).Error; err != nil {
		return model.PanelSettingDB{}, err
	}
	return p, nil
}

func (s *SQLiteStore) RotateAPIToken(token string) (model.PanelSettingDB, error) {
	p, err := s.GetPanelSetting()
	if err != nil {
		return model.PanelSettingDB{}, err
	}
	p.APIToken = token
	if err := s.db.Save(&p).Error; err != nil {
		return model.PanelSettingDB{}, err
	}
	return p, nil
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

func (s *SQLiteStore) ChangeUserPassword(username, oldPassword, newPassword string) (bool, error) {
	var u model.UserDB
	err := s.db.Where("username = ?", username).First(&u).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	if u.Password != oldPassword {
		return false, nil
	}
	u.Password = newPassword
	if err := s.db.Save(&u).Error; err != nil {
		return false, err
	}
	return true, nil
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
			ID:               int64(r.ID),
			Remark:           r.Remark,
			Port:             r.Port,
			Protocol:         r.Protocol,
			Password:         r.Password,
			UUID:             r.UUID,
			Email:            r.Email,
			Method:           r.Method,
			Flow:             r.Flow,
			Network:          r.Network,
			Security:         r.Security,
			SNI:              r.SNI,
			Host:             r.Host,
			Path:             r.Path,
			RealityDest:      r.RealityDest,
			ShortID:          r.ShortID,
			PublicKey:        r.PublicKey,
			PrivateKey:       r.PrivateKey,
			Enable:           r.Enable,
			Settings:         settings,
			Stream:           stream,
			SniffingEnabled:  r.SniffingEnabled,
			SniffingOverride: r.SniffingOverride,
			CreateUnix:       r.CreatedAt.Unix(),
			UpdateUnix:       r.UpdatedAt.Unix(),
		})
	}
	return out, nil
}

func (s *SQLiteStore) ListInboundsLite(limit, offset int) ([]model.InboundLite, int64, error) {
	var total int64
	if err := s.db.Model(&model.InboundDB{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := s.db.Model(&model.InboundDB{}).Select("id, remark, port, protocol, network, security, sni, enable, created_at, updated_at").Order("id asc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var rows []model.InboundDB
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]model.InboundLite, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.InboundLite{
			ID:         int64(r.ID),
			Remark:     r.Remark,
			Port:       r.Port,
			Protocol:   r.Protocol,
			Network:    r.Network,
			Security:   r.Security,
			SNI:        r.SNI,
			Enable:     r.Enable,
			CreateUnix: r.CreatedAt.Unix(),
			UpdateUnix: r.UpdatedAt.Unix(),
		})
	}
	return out, total, nil
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
		ID:               int64(r.ID),
		Remark:           r.Remark,
		Port:             r.Port,
		Protocol:         r.Protocol,
		Password:         r.Password,
		UUID:             r.UUID,
		Email:            r.Email,
		Method:           r.Method,
		Flow:             r.Flow,
		Network:          r.Network,
		Security:         r.Security,
		SNI:              r.SNI,
		Host:             r.Host,
		Path:             r.Path,
		RealityDest:      r.RealityDest,
		ShortID:          r.ShortID,
		PublicKey:        r.PublicKey,
		PrivateKey:       r.PrivateKey,
		Enable:           r.Enable,
		Settings:         settings,
		Stream:           stream,
		SniffingEnabled:  r.SniffingEnabled,
		SniffingOverride: r.SniffingOverride,
		CreateUnix:       r.CreatedAt.Unix(),
		UpdateUnix:       r.UpdatedAt.Unix(),
	}, true, nil
}

func (s *SQLiteStore) AddInbound(in model.Inbound) (model.Inbound, error) {
	settings, _ := json.Marshal(in.Settings)
	stream, _ := json.Marshal(in.Stream)
	row := model.InboundDB{
		Remark:           in.Remark,
		Port:             in.Port,
		Protocol:         in.Protocol,
		Password:         in.Password,
		UUID:             in.UUID,
		Email:            in.Email,
		Method:           in.Method,
		Flow:             in.Flow,
		Network:          in.Network,
		Security:         in.Security,
		SNI:              in.SNI,
		Host:             in.Host,
		Path:             in.Path,
		RealityDest:      in.RealityDest,
		ShortID:          in.ShortID,
		PublicKey:        in.PublicKey,
		PrivateKey:       in.PrivateKey,
		Enable:           in.Enable,
		Settings:         string(settings),
		Stream:           string(stream),
		Tag:              fmt.Sprintf("inbound-%d", time.Now().UnixNano()),
		SniffingEnabled:  in.SniffingEnabled,
		SniffingOverride: in.SniffingOverride,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(map[string]any{
			"sniffing_enabled":  in.SniffingEnabled,
			"sniffing_override": in.SniffingOverride,
		}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return model.Inbound{}, err
	}
	in.ID = int64(row.ID)
	in.Enable = row.Enable
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
	row.UUID = in.UUID
	row.Email = in.Email
	row.Method = in.Method
	row.Flow = in.Flow
	row.Network = in.Network
	row.Security = in.Security
	row.SNI = in.SNI
	row.Host = in.Host
	row.Path = in.Path
	row.RealityDest = in.RealityDest
	row.ShortID = in.ShortID
	row.PublicKey = in.PublicKey
	row.PrivateKey = in.PrivateKey
	row.Settings = string(settings)
	row.Stream = string(stream)
	row.SniffingEnabled = in.SniffingEnabled
	row.SniffingOverride = in.SniffingOverride
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

func (s *SQLiteStore) ListForwards() ([]model.Forward, error) {
	var rows []model.ForwardDB
	if err := s.db.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]model.Forward, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.Forward{ID: int64(r.ID), Remark: r.Remark, ListenPort: r.ListenPort, TargetHost: r.TargetHost, TargetPort: r.TargetPort, Protocol: r.Protocol, Enable: r.Enable})
	}
	return out, nil
}

func (s *SQLiteStore) AddForward(f model.Forward) (model.Forward, error) {
	row := model.ForwardDB{Remark: f.Remark, ListenPort: f.ListenPort, TargetHost: f.TargetHost, TargetPort: f.TargetPort, Protocol: f.Protocol, Enable: true}
	if err := s.db.Create(&row).Error; err != nil {
		return model.Forward{}, err
	}
	f.ID = int64(row.ID)
	f.Enable = true
	return f, nil
}

func (s *SQLiteStore) UpdateForward(id int64, f model.Forward) (model.Forward, bool, error) {
	var row model.ForwardDB
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.Forward{}, false, nil
		}
		return model.Forward{}, false, err
	}
	row.Remark = f.Remark
	row.ListenPort = f.ListenPort
	row.TargetHost = f.TargetHost
	row.TargetPort = f.TargetPort
	row.Protocol = f.Protocol
	if err := s.db.Save(&row).Error; err != nil {
		return model.Forward{}, false, err
	}
	return model.Forward{ID: int64(row.ID), Remark: row.Remark, ListenPort: row.ListenPort, TargetHost: row.TargetHost, TargetPort: row.TargetPort, Protocol: row.Protocol, Enable: row.Enable}, true, nil
}

func (s *SQLiteStore) ToggleForward(id int64) (model.Forward, bool, error) {
	var row model.ForwardDB
	if err := s.db.First(&row, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.Forward{}, false, nil
		}
		return model.Forward{}, false, err
	}
	row.Enable = !row.Enable
	if err := s.db.Save(&row).Error; err != nil {
		return model.Forward{}, false, err
	}
	return model.Forward{ID: int64(row.ID), Remark: row.Remark, ListenPort: row.ListenPort, TargetHost: row.TargetHost, TargetPort: row.TargetPort, Protocol: row.Protocol, Enable: row.Enable}, true, nil
}

func (s *SQLiteStore) DeleteForward(id int64) (bool, error) {
	res := s.db.Delete(&model.ForwardDB{}, id)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (s *SQLiteStore) NextInboundPort(base int) (int, error) {
	if base <= 0 {
		base = 20000
	}
	rows, err := s.ListInbounds()
	if err != nil {
		return 0, err
	}
	used := map[int]bool{}
	for _, r := range rows {
		used[r.Port] = true
	}
	p := base
	for used[p] {
		p++
	}
	return p, nil
}

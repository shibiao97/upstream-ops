package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Relays struct{ db *gorm.DB }

func NewRelays(db *gorm.DB) *Relays { return &Relays{db: db} }

func (r *Relays) FindConfig() (*RelayConfig, error) {
	var cfg RelayConfig
	err := r.db.Order("id ASC").First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *Relays) SaveConfig(cfg *RelayConfig, multipliers []RelayAccountMultiplier) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if cfg.ID == 0 {
			cfg.ID = 1
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(cfg).Error; err != nil {
			return err
		}
		for i := range multipliers {
			multipliers[i].ConfigID = cfg.ID
			if multipliers[i].Multiplier <= 0 {
				multipliers[i].Multiplier = 1
			}
		}
		if len(multipliers) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "config_id"}, {Name: "account_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "multiplier", "updated_at"}),
		}).Create(&multipliers).Error
	})
}

func (r *Relays) ListMultipliers(configID uint) ([]RelayAccountMultiplier, error) {
	var list []RelayAccountMultiplier
	err := r.db.Where("config_id = ?", configID).Order("account_id ASC").Find(&list).Error
	return list, err
}

func (r *Relays) SetCheckResult(configID uint, errMsg string) error {
	return r.db.Model(&RelayConfig{}).Where("id = ?", configID).Updates(map[string]any{
		"last_checked_at": gorm.Expr("CURRENT_TIMESTAMP"),
		"last_error":      errMsg,
	}).Error
}

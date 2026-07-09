package storage

import (
	"errors"

	"gorm.io/gorm"
)

type UserSchedulerSettings struct{ db *gorm.DB }

func NewUserSchedulerSettings(db *gorm.DB) *UserSchedulerSettings {
	return &UserSchedulerSettings{db: db}
}

func (r *UserSchedulerSettings) Get(userID uint) (*UserSchedulerSetting, error) {
	var s UserSchedulerSetting
	err := r.db.First(&s, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *UserSchedulerSettings) Upsert(userID uint, configJSON string) error {
	var s UserSchedulerSetting
	err := r.db.First(&s, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(&UserSchedulerSetting{UserID: userID, ConfigJSON: configJSON}).Error
	}
	if err != nil {
		return err
	}
	return r.db.Model(&s).Update("config_json", configJSON).Error
}

func (r *UserSchedulerSettings) List() ([]UserSchedulerSetting, error) {
	var list []UserSchedulerSetting
	return list, r.db.Find(&list).Error
}

package storage

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const SuperAdminUsername = "13153857932@163.com"

type Users struct{ db *gorm.DB }

func NewUsers(db *gorm.DB) *Users { return &Users{db: db} }

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (r *Users) BootstrapSuperAdmin(password string) (*SystemUser, error) {
	username := SuperAdminUsername
	var u SystemUser
	err := r.db.Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		hash, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		u = SystemUser{Username: username, PasswordHash: hash, Role: UserRoleSuperAdmin, Enabled: true}
		return &u, r.db.Create(&u).Error
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"role": UserRoleSuperAdmin, "enabled": true}
	if strings.TrimSpace(password) != "" && !CheckPassword(u.PasswordHash, password) {
		hash, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		updates["password_hash"] = hash
	}
	if err := r.db.Model(&u).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.FindByUsername(username)
}

func (r *Users) Create(username, password string) (*SystemUser, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	u := &SystemUser{
		Username:     strings.TrimSpace(username),
		PasswordHash: hash,
		Role:         UserRoleUser,
		Enabled:      true,
	}
	return u, r.db.Create(u).Error
}

func (r *Users) FindByUsername(username string) (*SystemUser, error) {
	var u SystemUser
	if err := r.db.Where("username = ?", strings.TrimSpace(username)).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Users) FindByID(id uint) (*SystemUser, error) {
	var u SystemUser
	if err := r.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Users) List(search string) ([]SystemUser, error) {
	q := r.db.Order("role ASC").Order("id ASC")
	search = strings.TrimSpace(search)
	if search != "" {
		q = q.Where("username LIKE ?", "%"+search+"%")
	}
	var list []SystemUser
	return list, q.Find(&list).Error
}

func (r *Users) SetEnabled(id uint, enabled bool) error {
	return r.db.Model(&SystemUser{}).
		Where("id = ? AND role <> ?", id, UserRoleSuperAdmin).
		Update("enabled", enabled).Error
}

func (r *Users) DeleteCascade(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var u SystemUser
		if err := tx.First(&u, id).Error; err != nil {
			return err
		}
		if u.Role == UserRoleSuperAdmin {
			return errors.New("不能删除超级管理员")
		}

		var channels []Channel
		if err := tx.Where("owner_user_id = ?", id).Find(&channels).Error; err != nil {
			return err
		}
		for _, ch := range channels {
			if err := deleteChannelCascade(tx, ch.ID); err != nil {
				return err
			}
		}

		var notifyIDs []uint
		if err := tx.Model(&NotificationChannel{}).Where("owner_user_id = ?", id).Pluck("id", &notifyIDs).Error; err != nil {
			return err
		}
		if len(notifyIDs) > 0 {
			if err := tx.Where("channel_id IN ?", notifyIDs).Delete(&NotificationLog{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", notifyIDs).Delete(&NotificationChannel{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", id).Delete(&UserSchedulerSetting{}).Error; err != nil {
			return err
		}
		return tx.Delete(&SystemUser{}, id).Error
	})
}

func (r *Users) AssignLegacyOwners(superID uint) error {
	if superID == 0 {
		return nil
	}
	if err := r.db.Model(&Channel{}).Where("owner_user_id = 0").Update("owner_user_id", superID).Error; err != nil {
		return err
	}
	return r.db.Model(&NotificationChannel{}).Where("owner_user_id = 0").Update("owner_user_id", superID).Error
}

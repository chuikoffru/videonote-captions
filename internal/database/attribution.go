package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"videonote-captions/internal/attribution"
)

// StartEvent records each valid start, including direct visits.
type StartEvent struct {
	ID        uint  `gorm:"primaryKey"`
	UserID    int64 `gorm:"index"`
	Source    string
	Language  string
	CreatedAt time.Time
}

// SourceStats contains aggregate conversion counts, without user identifiers.
type SourceStats struct {
	Source    string
	Users     int64
	Activated int64
	PaidUsers int64
}

// RecordStart serializes attribution with billing and quota writes on the user.
// INSERT ON CONFLICT also handles simultaneous first visits for an unknown user.
func (d *DB) RecordStart(userID int64, source, language string) error {
	if !attribution.ValidSource(source) {
		return fmt.Errorf("invalid start source")
	}
	if language != "ru" && language != "en" {
		return fmt.Errorf("unsupported language")
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "telegram_id"}}, DoNothing: true}).Create(&User{TelegramID: userID}).Error; e != nil {
			return e
		}
		var u User
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("telegram_id = ?", userID).First(&u).Error; e != nil {
			return e
		}
		now := time.Now()
		updates := map[string]interface{}{"last_source": source}
		if u.FirstStartedAt == nil {
			updates["first_source"] = source
			updates["first_started_at"] = now
		}
		// An explicit language choice before /start is also preserved.
		if u.Language == "" {
			updates["language"] = language
		}
		if e := tx.Model(&u).Updates(updates).Error; e != nil {
			return e
		}
		return tx.Create(&StartEvent{UserID: userID, Source: source, Language: language, CreatedAt: now}).Error
	})
}
func (d *DB) GetUserLanguage(userID int64) (string, error) {
	var u User
	e := d.Where("telegram_id = ?", userID).First(&u).Error
	return u.Language, e
}
func (d *DB) SetUserLanguage(userID int64, language string) error {
	if language != "ru" && language != "en" {
		return fmt.Errorf("unsupported language")
	}
	result := d.Model(&User{}).Where("telegram_id = ?", userID).Update("language", language)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (d *DB) GetSourceStats() ([]SourceStats, error) {
	var stats []SourceStats
	// EXISTS avoids multiplying videos by purchases when one user has many of both.
	err := d.Raw(`SELECT COALESCE(NULLIF(u.first_source, ''), 'unknown') AS source,
 COUNT(*) AS users,
 COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM video_logs v WHERE v.user_id = u.telegram_id AND v.video_file_id <> '' AND (u.first_started_at IS NULL OR v.created_at >= u.first_started_at))) AS activated,
 COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM credit_transactions c WHERE c.user_id = u.telegram_id AND c.type = 'purchase' AND c.amount > 0 AND (u.first_started_at IS NULL OR c.created_at >= u.first_started_at))) AS paid_users
 FROM users u GROUP BY COALESCE(NULLIF(u.first_source, ''), 'unknown') ORDER BY users DESC, source`).Scan(&stats).Error
	return stats, err
}

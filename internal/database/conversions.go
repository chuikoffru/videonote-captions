package database

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var ErrPlainQuotaExceeded = errors.New("daily plain circle quota exhausted")
var ErrInsufficientCredits = errors.New("insufficient credits")

type PlainConversion struct {
	ID        string `gorm:"primaryKey"`
	UserID    int64  `gorm:"index:plain_user_day"`
	Day       string `gorm:"index:plain_user_day;size:10"`
	State     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func plainQuotaDay(now time.Time) string {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("Europe/Moscow", 3*3600)
	}
	return now.In(loc).Format("2006-01-02")
}

func (d *DB) ReservePlainConversion(userID int64, limit int) (string, error) {
	if limit <= 0 {
		return "", ErrPlainQuotaExceeded
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(random[:])
	err := d.Transaction(func(tx *gorm.DB) error {
		var u User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("telegram_id = ?", userID).First(&u).Error; err != nil {
			return err
		}
		day := plainQuotaDay(time.Now())
		var count int64
		if err := tx.Model(&PlainConversion{}).Where("user_id = ? AND day = ? AND state IN ?", userID, day, []string{"pending", "delivered"}).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return ErrPlainQuotaExceeded
		}
		return tx.Create(&PlainConversion{ID: id, UserID: userID, Day: day, State: "pending"}).Error
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// A finished reservation is immutable: retries cannot refund a delivered result.
func (d *DB) FinishPlainConversion(id string, delivered bool) error {
	var reservation PlainConversion
	if err := d.First(&reservation, "id = ?", id).Error; err != nil {
		return err
	}
	state := "failed"
	if delivered {
		state = "delivered"
	}
	return d.Model(&PlainConversion{}).Where("id = ? AND state = ?", id, "pending").Update("state", state).Error
}

func (d *DB) LogVideoResult(userID int64, duration int, fileID, operation, sourceKind string, free bool) error {
	if operation != "plain_circle" && operation != "captioned_note" {
		return errors.New("invalid video operation")
	}
	if sourceKind != "video" && sourceKind != "video_note" {
		return errors.New("invalid video source")
	}
	return d.Transaction(func(tx *gorm.DB) error {
		paid := operation == "captioned_note" && !free
		if err := tx.Create(&VideoLog{UserID: userID, Duration: duration, VideoFileID: fileID, Operation: operation, SourceKind: sourceKind, IsPaid: paid}).Error; err != nil {
			return err
		}
		field := "total_plain_circles"
		if operation == "captioned_note" {
			field = "total_paid_videos"
			if free {
				field = "total_free_videos"
			}
		}
		return tx.Model(&Stats{}).Where("id = 1").Updates(map[string]interface{}{"total_video_notes": gorm.Expr("total_video_notes + 1"), field: gorm.Expr(field + " + 1")}).Error
	})
}

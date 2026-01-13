package database

import (
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// User представляет пользователя бота
type User struct {
	ID         uint  `gorm:"primaryKey"`
	TelegramID int64 `gorm:"uniqueIndex"`
	Username   string
	FirstName  string
	LastName   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// VideoLog хранит лог созданных видео
type VideoLog struct {
	ID          uint `gorm:"primaryKey"`
	UserID      int64
	Duration    int
	IsPaid      bool
	VideoFileID string // Telegram file_id отправленного видео
	CreatedAt   time.Time
}

// Stats хранит агрегированную статистику
type Stats struct {
	ID                   uint `gorm:"primaryKey"`
	TotalFreeVideos      int  // Видео до FREE_SECONDS
	TotalPaidVideos      int  // Оплаченные видео
	TotalPaymentRequests int  // Запрошенные платежи (инвойсы)
	UpdatedAt            time.Time
}

// DB обёртка над gorm.DB
type DB struct {
	*gorm.DB
}

// New создаёт и инициализирует базу данных
func New(dbPath string) (*DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	// Автомиграция
	if err := db.AutoMigrate(&User{}, &VideoLog{}, &Stats{}); err != nil {
		return nil, err
	}

	// Создаём единственную запись Stats если её нет
	var stats Stats
	if err := db.First(&stats).Error; err != nil {
		db.Create(&Stats{})
	}

	log.Println("✅ База данных инициализирована")
	return &DB{db}, nil
}

// GetOrCreateUser получает или создаёт пользователя
func (d *DB) GetOrCreateUser(telegramID int64, username, firstName, lastName string) (*User, error) {
	var user User
	err := d.Where("telegram_id = ?", telegramID).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		user = User{
			TelegramID: telegramID,
			Username:   username,
			FirstName:  firstName,
			LastName:   lastName,
		}
		if err := d.Create(&user).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}
	if err != nil {
		return nil, err
	}

	// Обновляем данные если изменились
	updates := map[string]interface{}{}
	if user.Username != username {
		updates["username"] = username
	}
	if user.FirstName != firstName {
		updates["first_name"] = firstName
	}
	if user.LastName != lastName {
		updates["last_name"] = lastName
	}
	if len(updates) > 0 {
		d.Model(&user).Updates(updates)
	}

	return &user, nil
}

// LogFreeVideo логирует бесплатное видео
func (d *DB) LogFreeVideo(userID int64, duration int, videoFileID string) error {
	// Создаём лог
	log := VideoLog{
		UserID:      userID,
		Duration:    duration,
		IsPaid:      false,
		VideoFileID: videoFileID,
	}
	if err := d.Create(&log).Error; err != nil {
		return err
	}

	// Обновляем статистику
	return d.Model(&Stats{}).Where("id = 1").Update("total_free_videos", gorm.Expr("total_free_videos + 1")).Error
}

// LogPaidVideo логирует платное видео
func (d *DB) LogPaidVideo(userID int64, duration int, videoFileID string) error {
	// Создаём лог
	log := VideoLog{
		UserID:      userID,
		Duration:    duration,
		IsPaid:      true,
		VideoFileID: videoFileID,
	}
	if err := d.Create(&log).Error; err != nil {
		return err
	}

	// Обновляем статистику
	return d.Model(&Stats{}).Where("id = 1").Update("total_paid_videos", gorm.Expr("total_paid_videos + 1")).Error
}

// LogPaymentRequest логирует запрос оплаты (отправленный инвойс)
func (d *DB) LogPaymentRequest() error {
	return d.Model(&Stats{}).Where("id = 1").Update("total_payment_requests", gorm.Expr("total_payment_requests + 1")).Error
}

// GetStats возвращает статистику
func (d *DB) GetStats() (*Stats, int64, error) {
	var stats Stats
	if err := d.First(&stats).Error; err != nil {
		return nil, 0, err
	}

	var userCount int64
	if err := d.Model(&User{}).Count(&userCount).Error; err != nil {
		return nil, 0, err
	}

	return &stats, userCount, nil
}

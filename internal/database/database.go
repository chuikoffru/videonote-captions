package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type User struct {
	ID                  uint  `gorm:"primaryKey"`
	TelegramID          int64 `gorm:"uniqueIndex"`
	Username            string
	FirstName           string
	LastName            string
	Credits             int64     `gorm:"default:0"` // Баланс кредитов
	FreeVideoNotesUsed  int       `gorm:"default:0"` // Использовано бесплатных кружочков в этом месяце
	FreeVideoNotesReset time.Time // Дата последнего сброса счетчика
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// CreditTransaction хранит историю операций с кредитами
type CreditTransaction struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       int64  `gorm:"index"` // Telegram ID пользователя
	Amount       int64  // Количество кредитов (положительное - пополнение, отрицательное - списание)
	Type         string // "purchase", "spend", "refund", "bonus"
	Description  string // Описание операции
	PaymentID    string `gorm:"uniqueIndex:payment_id_unique,where:payment_id <> ''"` // ID платежа (для пополнений)
	BalanceAfter int64  // Баланс после операции
	CreatedAt    time.Time
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
	TotalFreeVideos      int  // Бесплатные кружочки
	TotalPaidVideos      int  // Оплаченные видео
	TotalVideoNotes      int  `gorm:"default:0"` // Видео-кружочки
	TotalPaymentRequests int  // Запрошенные платежи (инвойсы)
	UpdatedAt            time.Time
}

// PaymentOrder persists invoices across bot restarts.
type PaymentOrder struct {
	Payload   string `gorm:"primaryKey"`
	UserID    int64
	Amount    int
	PriceRub  int
	Paid      bool
	CreatedAt time.Time
}

// DB обёртка над gorm.DB
type DB struct {
	*gorm.DB
}

// New создаёт и инициализирует базу данных
func New(dbPath string) (*DB, error) {
	db, err := gorm.Open(postgres.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	// Автомиграция
	if err := db.AutoMigrate(&User{}, &VideoLog{}, &Stats{}, &PaymentOrder{}, &CreditTransaction{}); err != nil {
		return nil, err
	}

	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Stats{ID: 1}).Error; err != nil {
		return nil, err
	}

	log.Println("✅ База данных инициализирована")
	return &DB{db}, nil
}

// GetOrCreateUser получает или создаёт пользователя
func (d *DB) GetOrCreateUser(telegramID int64, username, firstName, lastName string) (*User, error) {
	u := User{TelegramID: telegramID, Username: username, FirstName: firstName, LastName: lastName}
	err := d.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "telegram_id"}}, DoUpdates: clause.AssignmentColumns([]string{"username", "first_name", "last_name"})}).Create(&u).Error
	return &u, err
}

// GetAllUsers возвращает список Telegram ID всех пользователей
func (d *DB) GetAllUsers() ([]int64, error) {
	var ids []int64
	err := d.Model(&User{}).Pluck("telegram_id", &ids).Error
	return ids, err
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

// GetUserCredits возвращает баланс кредитов пользователя
func (d *DB) GetUserCredits(telegramID int64) (int64, error) {
	var user User
	err := d.Where("telegram_id = ?", telegramID).First(&user).Error
	if err != nil {
		return 0, err
	}
	return user.Credits, nil
}

// GetCreditTransactions возвращает историю транзакций пользователя
func (d *DB) GetCreditTransactions(telegramID int64, limit int) ([]CreditTransaction, error) {
	var transactions []CreditTransaction
	err := d.Where("user_id = ?", telegramID).Order("created_at DESC").Limit(limit).Find(&transactions).Error
	return transactions, err
}

// FullStats расширенная статистика для админа
type FullStats struct {
	// По пользователям
	TotalUsers      int64
	UsersNoVideos   int64
	UsersOneVideo   int64
	UsersMoreVideos int64

	// По типам видео
	VideoNotes int // Видео-кружочки

	// По генерациям
	FreeVideos      int
	PaymentRequests int
	PaidVideos      int

	// По деньгам
	TotalBalances int64
	TotalSpent    int64
}

// GetFullStats возвращает полную статистику для админа
func (d *DB) GetFullStats() (*FullStats, error) {
	result := &FullStats{}

	// 1. Получаем статистику по пользователям
	if err := d.Model(&User{}).Count(&result.TotalUsers).Error; err != nil {
		return nil, err
	}

	// 2. Получаем количество видео на каждого пользователя
	type UserVideoCount struct {
		UserID int64
		Count  int64
	}
	var videoCountResults []UserVideoCount
	if err := d.Model(&VideoLog{}).Select("user_id, count(*) as count").Group("user_id").Scan(&videoCountResults).Error; err != nil {
		return nil, err
	}

	var usersWithVideos int64 = 0
	for _, res := range videoCountResults {
		usersWithVideos++
		if res.Count == 1 {
			result.UsersOneVideo++
		} else if res.Count > 1 {
			result.UsersMoreVideos++
		}
	}
	result.UsersNoVideos = result.TotalUsers - usersWithVideos
	if result.UsersNoVideos < 0 {
		result.UsersNoVideos = 0
	}

	// 3. Статистика из таблицы Stats
	var stats Stats
	if err := d.First(&stats).Error; err == nil {
		result.VideoNotes = stats.TotalVideoNotes
		result.FreeVideos = stats.TotalFreeVideos
		result.PaymentRequests = stats.TotalPaymentRequests
		result.PaidVideos = stats.TotalPaidVideos
	}

	// 5. Сумма балансов пользователей
	type SumResult struct {
		Total int64
	}
	var balanceSum SumResult
	if err := d.Model(&User{}).Select("COALESCE(SUM(credits), 0) as total").Scan(&balanceSum).Error; err != nil {
		return nil, err
	}
	result.TotalBalances = balanceSum.Total

	// 6. Сумма использованных средств (spend транзакции)
	var spentSum SumResult
	if err := d.Model(&CreditTransaction{}).Where("type IN ?", []string{"spend", "refund"}).Select("COALESCE(SUM(-amount), 0) as total").Scan(&spentSum).Error; err != nil {
		return nil, err
	}
	result.TotalSpent = spentSum.Total

	return result, nil
}

// GetFreeVideoNotesRemaining возвращает количество оставшихся бесплатных кружочков
// и автоматически сбрасывает счетчик если наступил новый месяц
func (d *DB) GetFreeVideoNotesRemaining(userID int64, limit int) (remaining int, err error) {
	err = d.Transaction(func(tx *gorm.DB) error {
		var u User
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("telegram_id = ?", userID).First(&u).Error; e != nil {
			return e
		}
		now := time.Now()
		if u.FreeVideoNotesReset.Year() != now.Year() || u.FreeVideoNotesReset.Month() != now.Month() {
			u.FreeVideoNotesUsed = 0
			u.FreeVideoNotesReset = now
			if e := tx.Save(&u).Error; e != nil {
				return e
			}
		}
		remaining = limit - u.FreeVideoNotesUsed
		if remaining < 0 {
			remaining = 0
		}
		return nil
	})
	return
}

// ReserveNote serializes quota/balance updates for one user.
func (d *DB) ReserveNote(userID int64, limit, cost int, admin bool) (free bool, err error) {
	err = d.Transaction(func(tx *gorm.DB) error {
		var u User
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("telegram_id = ?", userID).First(&u).Error; e != nil {
			return e
		}
		now := time.Now()
		if u.FreeVideoNotesReset.Year() != now.Year() || u.FreeVideoNotesReset.Month() != now.Month() {
			u.FreeVideoNotesUsed = 0
			u.FreeVideoNotesReset = now
		}
		free = admin || u.FreeVideoNotesUsed < limit
		if !admin {
			if free {
				u.FreeVideoNotesUsed++
			} else {
				if u.Credits < int64(cost) {
					return fmt.Errorf("insufficient credits")
				}
				u.Credits -= int64(cost)
				if e := tx.Create(&CreditTransaction{UserID: userID, Amount: -int64(cost), Type: "spend", Description: "Video note", BalanceAfter: u.Credits}).Error; e != nil {
					return e
				}
			}
		}
		return tx.Save(&u).Error
	})
	return
}

func (d *DB) RefundNote(userID int64, free bool, cost int, admin bool) error {
	if admin {
		return nil
	}
	return d.Transaction(func(tx *gorm.DB) error {
		var u User
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("telegram_id = ?", userID).First(&u).Error; e != nil {
			return e
		}
		if free {
			if u.FreeVideoNotesUsed > 0 {
				u.FreeVideoNotesUsed--
			}
		} else {
			u.Credits += int64(cost)
			if e := tx.Create(&CreditTransaction{UserID: userID, Amount: int64(cost), Type: "refund", Description: "Failed video note", BalanceAfter: u.Credits}).Error; e != nil {
				return e
			}
		}
		return tx.Save(&u).Error
	})
}
func (d *DB) LogNote(userID int64, duration int, fileID string, free bool) error {
	return d.Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&VideoLog{UserID: userID, Duration: duration, VideoFileID: fileID, IsPaid: !free}).Error; e != nil {
			return e
		}
		field := "total_paid_videos"
		if free {
			field = "total_free_videos"
		}
		return tx.Model(&Stats{}).Where("id = 1").Updates(map[string]interface{}{"total_video_notes": gorm.Expr("total_video_notes + 1"), field: gorm.Expr(field + " + 1")}).Error
	})
}
func (d *DB) CreateOrder(p PaymentOrder) error { return d.Create(&p).Error }
func (d *DB) GetOrder(payload string) (*PaymentOrder, error) {
	var p PaymentOrder
	err := d.Where("payload = ?", payload).First(&p).Error
	return &p, err
}

// CompleteOrder is idempotent: Telegram may deliver a payment more than once.
func (d *DB) CompleteOrder(payload string, userID int64, total int, currency, chargeID string) error {
	return d.Transaction(func(tx *gorm.DB) error {
		var p PaymentOrder
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("payload = ?", payload).First(&p).Error; e != nil {
			return e
		}
		if p.UserID != userID || total != p.PriceRub*100 || currency != "RUB" || chargeID == "" {
			return fmt.Errorf("payment mismatch")
		}
		if p.Paid {
			return nil
		}
		var u User
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("telegram_id = ?", userID).First(&u).Error; e != nil {
			return e
		}
		u.Credits += int64(p.Amount)
		if e := tx.Save(&u).Error; e != nil {
			return e
		}
		if e := tx.Create(&CreditTransaction{UserID: userID, Amount: int64(p.Amount), Type: "purchase", PaymentID: chargeID, BalanceAfter: u.Credits}).Error; e != nil {
			return e
		}
		return tx.Model(&p).Update("paid", true).Error
	})
}

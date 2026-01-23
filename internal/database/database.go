package database

import (
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type User struct {
	ID                   uint  `gorm:"primaryKey"`
	TelegramID           int64 `gorm:"uniqueIndex"`
	Username             string
	FirstName            string
	LastName             string
	Credits              int64  `gorm:"default:0"` // Баланс кредитов
	HorizontalBackground string // Путь к кастомному горизонтальному фону
	VerticalBackground   string // Путь к кастомному вертикальному фону
	FreeVideoNotesUsed   int    `gorm:"default:0"` // Использовано бесплатных кружочков в этом месяце
	FreeVideoNotesReset  time.Time               // Дата последнего сброса счетчика
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CreditTransaction хранит историю операций с кредитами
type CreditTransaction struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       int64  `gorm:"index"` // Telegram ID пользователя
	Amount       int64  // Количество кредитов (положительное - пополнение, отрицательное - списание)
	Type         string // "purchase", "spend", "refund", "bonus"
	Description  string // Описание операции
	PaymentID    string // ID платежа (для пополнений)
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
	TotalFreeVideos      int  // Видео до FREE_SECONDS
	TotalPaidVideos      int  // Оплаченные видео
	TotalVerticalVideos  int  `gorm:"default:0"` // Вертикальные видео
	TotalVideoNotes      int  `gorm:"default:0"` // Видео-кружочки
	TotalPaymentRequests int  // Запрошенные платежи (инвойсы)
	UpdatedAt            time.Time
}

// VoiceCache хранит кэш обработанных голосовых сообщений
type VoiceCache struct {
	ID                   uint   `gorm:"primaryKey"`
	VoiceFileUniqueID    string `gorm:"uniqueIndex"` // file_unique_id голосового сообщения
	VideoFileID          string // file_id горизонтального видео
	VerticalVideoFileID  string // file_id вертикального видео (опционально)
	TranscriptionJSON    string // JSON результата транскрипции
	HorizontalBackground string // Путь к фону при генерации горизонтального видео
	VerticalBackground   string // Путь к фону при генерации вертикального видео
	CreatedAt            time.Time
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
	if err := db.AutoMigrate(&User{}, &VideoLog{}, &Stats{}, &VoiceCache{}, &CreditTransaction{}); err != nil {
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

// LogVerticalVideo логирует создание вертикального видео
func (d *DB) LogVerticalVideo() error {
	return d.Model(&Stats{}).Where("id = 1").Update("total_vertical_videos", gorm.Expr("total_vertical_videos + 1")).Error
}

// LogVideoNote логирует создание видео-кружочка
func (d *DB) LogVideoNote() error {
	return d.Model(&Stats{}).Where("id = 1").Update("total_video_notes", gorm.Expr("total_video_notes + 1")).Error
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

// GetCachedVideo ищет кэшированное видео по file_unique_id голосового сообщения
func (d *DB) GetCachedVideo(voiceFileUniqueID string) (*VoiceCache, error) {
	var cache VoiceCache
	err := d.Where("voice_file_unique_id = ?", voiceFileUniqueID).First(&cache).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cache, nil
}

// SaveVideoCache сохраняет кэш горизонтального видео
func (d *DB) SaveVideoCache(voiceFileUniqueID, videoFileID string) error {
	// Пробуем найти существующий кэш
	var cache VoiceCache
	err := d.Where("voice_file_unique_id = ?", voiceFileUniqueID).First(&cache).Error
	if err == gorm.ErrRecordNotFound {
		// Создаём новый
		cache = VoiceCache{
			VoiceFileUniqueID: voiceFileUniqueID,
			VideoFileID:       videoFileID,
		}
		return d.Create(&cache).Error
	}
	if err != nil {
		return err
	}
	// Обновляем существующий
	return d.Model(&cache).Update("video_file_id", videoFileID).Error
}

// SaveVerticalVideoCache сохраняет кэш вертикального видео
func (d *DB) SaveVerticalVideoCache(voiceFileUniqueID, verticalVideoFileID string) error {
	// Пробуем найти существующий кэш
	var cache VoiceCache
	err := d.Where("voice_file_unique_id = ?", voiceFileUniqueID).First(&cache).Error
	if err == gorm.ErrRecordNotFound {
		// Создаём новый
		cache = VoiceCache{
			VoiceFileUniqueID:   voiceFileUniqueID,
			VerticalVideoFileID: verticalVideoFileID,
		}
		return d.Create(&cache).Error
	}
	if err != nil {
		return err
	}
	// Обновляем существующий
	return d.Model(&cache).Update("vertical_video_file_id", verticalVideoFileID).Error
}

// GetUsageStats возвращает статистику использования бота пользователями
func (d *DB) GetUsageStats() (users0, users1, usersMore int64, err error) {
	// 1. Получаем общее количество пользователей
	var totalUsers int64
	if err := d.Model(&User{}).Count(&totalUsers).Error; err != nil {
		return 0, 0, 0, err
	}

	// 2. Получаем количество видео на каждого пользователя
	type Result struct {
		UserID int64
		Count  int64
	}
	var results []Result
	if err := d.Model(&VideoLog{}).Select("user_id, count(*) as count").Group("user_id").Scan(&results).Error; err != nil {
		return 0, 0, 0, err
	}

	// 3. Считаем категории
	var withVideoCount int64 = 0
	for _, res := range results {
		// Мы считаем уникальных пользователей, у которых есть видео.
		// Result.UserID - это telegram_id.
		// Если в video_logs есть записи для юзеров, которых нет в users (маловероятно, но возможно если база чистилась неравномерно),
		// это может немного исказить "0 videos", но в целом логика верная.

		withVideoCount++

		if res.Count == 1 {
			users1++
		} else if res.Count > 1 {
			usersMore++
		}
	}

	// Пользователи без видео = Всего пользователей - Пользователи с видео
	users0 = totalUsers - withVideoCount

	// Если вдруг video_logs содержит ID которых нет в users, users0 может стать отрицательным.
	// Нормализуем на всякий случай.
	if users0 < 0 {
		users0 = 0
	}

	return users0, users1, usersMore, nil
}

// UpdateUserBackground обновляет путь к фону пользователя
func (d *DB) UpdateUserBackground(telegramID int64, isHorizontal bool, path string) error {
	field := "vertical_background"
	if isHorizontal {
		field = "horizontal_background"
	}
	return d.Model(&User{}).Where("telegram_id = ?", telegramID).Update(field, path).Error
}

// GetUserBackgrounds возвращает пути к фонам пользователя
func (d *DB) GetUserBackgrounds(telegramID int64) (horizontal, vertical string, err error) {
	var user User
	err = d.Where("telegram_id = ?", telegramID).First(&user).Error
	if err != nil {
		return "", "", err
	}
	return user.HorizontalBackground, user.VerticalBackground, nil
}

// SaveCacheWithTranscription сохраняет кэш с транскрипцией и информацией о фоне
func (d *DB) SaveCacheWithTranscription(voiceFileUniqueID, videoFileID, transcriptionJSON, horizontalBg string) error {
	var cache VoiceCache
	err := d.Where("voice_file_unique_id = ?", voiceFileUniqueID).First(&cache).Error
	if err == gorm.ErrRecordNotFound {
		cache = VoiceCache{
			VoiceFileUniqueID:    voiceFileUniqueID,
			VideoFileID:          videoFileID,
			TranscriptionJSON:    transcriptionJSON,
			HorizontalBackground: horizontalBg,
		}
		return d.Create(&cache).Error
	}
	if err != nil {
		return err
	}
	return d.Model(&cache).Updates(map[string]interface{}{
		"video_file_id":         videoFileID,
		"transcription_json":    transcriptionJSON,
		"horizontal_background": horizontalBg,
	}).Error
}

// SaveVerticalCacheWithBackground сохраняет кэш вертикального видео с фоном
func (d *DB) SaveVerticalCacheWithBackground(voiceFileUniqueID, verticalVideoFileID, verticalBg string) error {
	var cache VoiceCache
	err := d.Where("voice_file_unique_id = ?", voiceFileUniqueID).First(&cache).Error
	if err == gorm.ErrRecordNotFound {
		cache = VoiceCache{
			VoiceFileUniqueID:   voiceFileUniqueID,
			VerticalVideoFileID: verticalVideoFileID,
			VerticalBackground:  verticalBg,
		}
		return d.Create(&cache).Error
	}
	if err != nil {
		return err
	}
	return d.Model(&cache).Updates(map[string]interface{}{
		"vertical_video_file_id": verticalVideoFileID,
		"vertical_background":    verticalBg,
	}).Error
}

// InvalidateVideoCache очищает file_id видео (при изменении фона), сохраняя транскрипцию
func (d *DB) InvalidateVideoCache(voiceFileUniqueID string, invalidateHorizontal, invalidateVertical bool) error {
	var cache VoiceCache
	err := d.Where("voice_file_unique_id = ?", voiceFileUniqueID).First(&cache).Error
	if err != nil {
		return err
	}

	updates := map[string]interface{}{}
	if invalidateHorizontal {
		updates["video_file_id"] = ""
		updates["horizontal_background"] = ""
	}
	if invalidateVertical {
		updates["vertical_video_file_id"] = ""
		updates["vertical_background"] = ""
	}

	if len(updates) > 0 {
		return d.Model(&cache).Updates(updates).Error
	}
	return nil
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

// AddCredits добавляет кредиты на баланс пользователя
func (d *DB) AddCredits(telegramID int64, amount int64, txType, description, paymentID string) error {
	return d.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Where("telegram_id = ?", telegramID).First(&user).Error; err != nil {
			return err
		}

		newBalance := user.Credits + amount

		// Обновляем баланс
		if err := tx.Model(&user).Update("credits", newBalance).Error; err != nil {
			return err
		}

		// Создаём запись транзакции
		transaction := CreditTransaction{
			UserID:       telegramID,
			Amount:       amount,
			Type:         txType,
			Description:  description,
			PaymentID:    paymentID,
			BalanceAfter: newBalance,
		}
		return tx.Create(&transaction).Error
	})
}

// SpendCredits списывает кредиты с баланса (возвращает ошибку если недостаточно)
func (d *DB) SpendCredits(telegramID int64, amount int64, description string) error {
	return d.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Where("telegram_id = ?", telegramID).First(&user).Error; err != nil {
			return err
		}

		if user.Credits < amount {
			return gorm.ErrRecordNotFound // Используем как индикатор недостаточного баланса
		}

		newBalance := user.Credits - amount

		// Обновляем баланс
		if err := tx.Model(&user).Update("credits", newBalance).Error; err != nil {
			return err
		}

		// Создаём запись транзакции
		transaction := CreditTransaction{
			UserID:       telegramID,
			Amount:       -amount, // Отрицательное значение для списания
			Type:         "spend",
			Description:  description,
			BalanceAfter: newBalance,
		}
		return tx.Create(&transaction).Error
	})
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
	VerticalVideos   int
	HorizontalVideos int
	VideoNotes       int // Видео-кружочки

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
		result.VerticalVideos = stats.TotalVerticalVideos
		result.VideoNotes = stats.TotalVideoNotes
		result.FreeVideos = stats.TotalFreeVideos
		result.PaymentRequests = stats.TotalPaymentRequests
		result.PaidVideos = stats.TotalPaidVideos
	}

	// 4. Горизонтальные = все видео - вертикальные (из VideoLog можно посчитать)
	var totalVideos int64
	if err := d.Model(&VideoLog{}).Count(&totalVideos).Error; err != nil {
		return nil, err
	}
	result.HorizontalVideos = int(totalVideos) - result.VerticalVideos
	if result.HorizontalVideos < 0 {
		result.HorizontalVideos = 0
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
	if err := d.Model(&CreditTransaction{}).Where("type = ?", "spend").Select("COALESCE(SUM(ABS(amount)), 0) as total").Scan(&spentSum).Error; err != nil {
		return nil, err
	}
	result.TotalSpent = spentSum.Total

	return result, nil
}

// GetFreeVideoNotesRemaining возвращает количество оставшихся бесплатных кружочков
// и автоматически сбрасывает счетчик если наступил новый месяц
func (d *DB) GetFreeVideoNotesRemaining(telegramID int64, monthlyLimit int) (int, error) {
	var user User
	if err := d.Where("telegram_id = ?", telegramID).First(&user).Error; err != nil {
		return 0, err
	}

	now := time.Now()
	// Проверяем, нужен ли сброс (новый месяц)
	if user.FreeVideoNotesReset.IsZero() ||
		now.Year() > user.FreeVideoNotesReset.Year() ||
		(now.Year() == user.FreeVideoNotesReset.Year() && now.Month() > user.FreeVideoNotesReset.Month()) {
		// Сбрасываем счетчик
		d.Model(&user).Updates(map[string]interface{}{
			"free_video_notes_used":  0,
			"free_video_notes_reset": now,
		})
		return monthlyLimit, nil
	}

	remaining := monthlyLimit - user.FreeVideoNotesUsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

// UseFreeVideoNote использует один бесплатный кружочек
func (d *DB) UseFreeVideoNote(telegramID int64) error {
	return d.Model(&User{}).Where("telegram_id = ?", telegramID).
		Update("free_video_notes_used", gorm.Expr("free_video_notes_used + 1")).Error
}

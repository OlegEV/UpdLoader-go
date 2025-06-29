package bot

import (
	"fmt"
	"io"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"

	"upd-loader-go/internal/config"
	"upd-loader-go/internal/processor"
)

// TelegramUPDBot represents Telegram bot for UPD processing
type TelegramUPDBot struct {
	config    *config.Config
	bot       *tgbotapi.BotAPI
	processor *processor.UPDProcessor
	logger    *logrus.Logger
}

// NewTelegramUPDBot creates a new Telegram UPD bot
func NewTelegramUPDBot(cfg *config.Config, logger *logrus.Logger) (*TelegramUPDBot, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %v", err)
	}

	processor := processor.NewUPDProcessor(cfg, logger)

	return &TelegramUPDBot{
		config:    cfg,
		bot:       bot,
		processor: processor,
		logger:    logger,
	}, nil
}

// Run starts the bot
func (b *TelegramUPDBot) Run() error {
	b.logger.Info("Starting Telegram bot...")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			go b.handleUpdate(update)
		}
	}

	return nil
}

// handleUpdate handles incoming updates
func (b *TelegramUPDBot) handleUpdate(update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Errorf("Panic in handleUpdate: %v", r)
		}
	}()

	userID := update.Message.From.ID

	if !b.config.IsAuthorizedUser(userID) {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ У вас нет доступа к этому боту.\nОбратитесь к администратору для получения доступа.")
		b.bot.Send(msg)
		return
	}

	if update.Message.IsCommand() {
		b.handleCommand(update)
	} else if update.Message.Document != nil {
		b.handleDocument(update)
	} else {
		b.handleText(update)
	}
}

// handleCommand handles bot commands
func (b *TelegramUPDBot) handleCommand(update tgbotapi.Update) {
	switch update.Message.Command() {
	case "start":
		b.handleStartCommand(update)
	case "help":
		b.handleHelpCommand(update)
	case "status":
		b.handleStatusCommand(update)
	default:
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❓ Неизвестная команда. Используйте /help для получения справки.")
		b.bot.Send(msg)
	}
}

// handleStartCommand handles /start command
func (b *TelegramUPDBot) handleStartCommand(update tgbotapi.Update) {
	welcomeMessage := `🤖 Добро пожаловать в бот загрузки УПД в МойСклад!

📋 Что я умею:
• Обрабатывать ZIP архивы с УПД документами
• Создавать счета-фактуры в МойСклад
• Предоставлять детальную информацию о результатах

📎 Просто отправьте мне ZIP файл с УПД, и я его обработаю!

ℹ️ Используйте /help для получения дополнительной информации.`

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, welcomeMessage)
	b.bot.Send(msg)
}

// handleHelpCommand handles /help command
func (b *TelegramUPDBot) handleHelpCommand(update tgbotapi.Update) {
	helpMessage := fmt.Sprintf(`📖 Справка по использованию бота

🔧 Доступные команды:
/start - Начать работу с ботом
/help - Показать эту справку
/status - Проверить статус подключения к МойСклад

📎 Как загрузить УПД:
1. Отправьте ZIP архив с УПД документом
2. Дождитесь обработки (обычно 10-30 секунд)
3. Получите результат с ссылкой на созданный документ

📋 Требования к файлам:
• Формат: ZIP архив
• Максимальный размер: %d МБ
• Содержимое: УПД в стандартном формате

❓ При возникновении проблем обратитесь к администратору.`, b.config.MaxFileSize/1024/1024)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, helpMessage)
	b.bot.Send(msg)
}

// handleStatusCommand handles /status command
func (b *TelegramUPDBot) handleStatusCommand(update tgbotapi.Update) {
	// Send checking message
	statusMsg := tgbotapi.NewMessage(update.Message.Chat.ID, "🔄 Проверяю подключение к МойСклад...")
	sentMsg, err := b.bot.Send(statusMsg)
	if err != nil {
		b.logger.Errorf("Failed to send status message: %v", err)
		return
	}

	// Check MoySkald status
	statusInfo := b.processor.GetMoySkaldStatus()

	var resultMessage string
	if success, ok := statusInfo["success"].(bool); ok && success {
		// Format detailed success message
		employee, _ := statusInfo["employee"].(map[string]interface{})
		organization, _ := statusInfo["organization"].(map[string]interface{})
		permissions, _ := statusInfo["permissions"].(map[string]interface{})

		employeeName, _ := employee["name"].(string)
		employeeEmail, _ := employee["email"].(string)
		orgName, _ := organization["name"].(string)
		orgINN, _ := organization["inn"].(string)

		canCreateInvoices, _ := permissions["can_create_invoices"].(bool)
		canAccessCounterparties, _ := permissions["can_access_counterparties"].(bool)
		organizationsCount, _ := permissions["organizations_count"].(float64)

		resultMessage = fmt.Sprintf(`✅ Статус системы: Все работает!

👤 Пользователь МойСклад:
   Имя: %s
   Email: %s

🏢 Организация:
   Название: %s
   ИНН: %s

🔐 Права доступа:
   %s Создание счетов-фактур
   %s Работа с контрагентами
   📊 Организаций: %.0f

🤖 Telegram бот: Активен
📁 Временная папка: Доступна

🎉 Готов к обработке УПД документов!`,
			employeeName, employeeEmail, orgName, orgINN,
			boolToEmoji(canCreateInvoices), boolToEmoji(canAccessCounterparties), organizationsCount)
	} else {
		// Format error message
		errorStr, _ := statusInfo["error"].(string)
		details, _ := statusInfo["details"].(string)

		resultMessage = fmt.Sprintf(`⚠️ Статус системы: Есть проблемы

❌ МойСклад API: %s
🤖 Telegram бот: Активен

📝 Детали: %s

💡 Рекомендации:
• Проверьте токен МойСклад API
• Убедитесь в наличии прав доступа
• Обратитесь к администратору`, errorStr, details)
	}

	// Edit the status message
	editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, sentMsg.MessageID, resultMessage)
	b.bot.Send(editMsg)
}

// handleDocument handles document uploads
func (b *TelegramUPDBot) handleDocument(update tgbotapi.Update) {
	userID := update.Message.From.ID
	document := update.Message.Document

	b.logger.Infof("Received document from user %d: %s", userID, document.FileName)

	// Send processing message
	processingMsg := tgbotapi.NewMessage(update.Message.Chat.ID,
		fmt.Sprintf(`📄 Получен файл: %s
🔄 Начинаю обработку УПД...

⏳ Это может занять до 30 секунд, пожалуйста, подождите.`, document.FileName))

	sentMsg, err := b.bot.Send(processingMsg)
	if err != nil {
		b.logger.Errorf("Failed to send processing message: %v", err)
		return
	}

	// Download file
	fileContent, err := b.downloadFile(document.FileID)
	if err != nil {
		b.logger.Errorf("Failed to download file: %v", err)
		errorMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, sentMsg.MessageID,
			"❌ Произошла ошибка при скачивании файла.\nПопробуйте еще раз или обратитесь к администратору.")
		b.bot.Send(errorMsg)
		return
	}

	// Process UPD
	result := b.processor.ProcessUPDFile(fileContent, document.FileName)

	// Send result
	editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, sentMsg.MessageID, result.Message)
	b.bot.Send(editMsg)

	if result.Success {
		b.logger.Infof("UPD successfully processed for user %d", userID)
	} else {
		b.logger.Warningf("UPD processing error for user %d: %s", userID, result.ErrorCode)
	}
}

// handleText handles text messages
func (b *TelegramUPDBot) handleText(update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID,
		`📎 Для обработки УПД отправьте мне ZIP архив с документом.

ℹ️ Используйте /help для получения подробной информации.`)
	b.bot.Send(msg)
}

// downloadFile downloads file from Telegram
func (b *TelegramUPDBot) downloadFile(fileID string) ([]byte, error) {
	file, err := b.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}

	fileURL := file.Link(b.bot.Token)

	resp, err := http.Get(fileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download file: status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %v", err)
	}

	return content, nil
}

// boolToEmoji converts boolean to emoji
func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}
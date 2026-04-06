package deliver

// Channel — интерфейс канала доставки.
// Telegram, WhatsApp, HTTP — всё реализует это.
type Channel interface {
	// Start запускает канал (long polling, webhook, etc.)
	Start() error
	// Stop останавливает канал.
	Stop()
	// Name возвращает имя канала.
	Name() string
}

// IncomingMessage — нормализованное входящее сообщение от любого канала.
type IncomingMessage struct {
	Text      string
	SessionID string // уникальный ID разговора (chat_id для Telegram, etc.)
	UserID    string // ID юзера
	UserName  string // имя юзера
	Channel   string // "telegram", "http", "whatsapp"
	ImageDesc string // описание изображения (если юзер прислал фото)
}

// OutgoingMessage — ответ для отправки в канал.
type OutgoingMessage struct {
	Text      string
	SessionID string
	Channel   string
}

// Handler обрабатывает входящее сообщение и возвращает ответ.
type Handler func(msg IncomingMessage) OutgoingMessage

// CommandResult — результат выполнения команды.
type CommandResult struct {
	Text    string
	OK      bool
	Buttons [][]InlineButton // inline keyboard rows
}

// InlineButton — кнопка для inline keyboard.
type InlineButton struct {
	Text         string
	CallbackData string
}

// CommandHandler обрабатывает команды бота (e.g., /compact, /provider).
// Возвращает nil если команда не обработана.
type CommandHandler func(command, sessionID string) *CommandResult

// CallbackHandler обрабатывает нажатия на inline кнопки.
// Returns text to show and optional new buttons.
type CallbackHandler func(callbackData, sessionID string) *CommandResult

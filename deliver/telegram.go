package deliver

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegoutil"
)

// пакетный контекст для простых вызовов
var bgCtx = context.Background()

// ImageDescriber describes images for the synth (Grok vision API).
type ImageDescriber interface {
	Analyze(imageData []byte, mimeType, prompt string) (string, error)
}

// Telegram — канал доставки через Telegram Bot API (telego).
type Telegram struct {
	bot             *telego.Bot
	token           string
	handler         Handler
	commandHandler  CommandHandler
	callbackHandler CallbackHandler
	imageDescriber  ImageDescriber
	stop            chan struct{}
	cancel          context.CancelFunc
	updates         <-chan telego.Update
}

func NewTelegram(token string, handler Handler, cmdHandler CommandHandler, cbHandler CallbackHandler, imgDescriber ImageDescriber) (*Telegram, error) {
	bot, err := telego.NewBot(token)
	if err != nil {
		return nil, err
	}

	me, err := bot.GetMe(bgCtx)
	if err != nil {
		return nil, err
	}

	log.Printf("telegram: authorized as @%s", me.Username)

	return &Telegram{
		bot:             bot,
		token:           token,
		handler:         handler,
		commandHandler:  cmdHandler,
		callbackHandler: cbHandler,
		imageDescriber:  imgDescriber,
		stop:            make(chan struct{}),
	}, nil
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Start() error {
	pollCtx, cancel := context.WithCancel(bgCtx)
	t.cancel = cancel
	updates, err := t.bot.UpdatesViaLongPolling(pollCtx, nil)
	if err != nil {
		return err
	}
	t.updates = updates

	go func() {
		for {
			select {
			case <-t.stop:
				return
			case update, ok := <-t.updates:
				if !ok {
					return
				}
				if update.Message != nil {
					go t.handleMessage(update.Message)
				}
				if update.CallbackQuery != nil {
					go t.handleCallback(update.CallbackQuery)
				}
			}
		}
	}()

	log.Printf("telegram: listening for messages")
	return nil
}

func (t *Telegram) Stop() {
	close(t.stop)
	if t.cancel != nil {
		t.cancel()
	}
	log.Printf("telegram: stopped")
}

func (t *Telegram) handleMessage(msg *telego.Message) {
	text := strings.TrimSpace(msg.Text)

	// Handle photos: extract caption + describe image
	var imageDesc string
	if len(msg.Photo) > 0 {
		text = strings.TrimSpace(msg.Caption)
		if text == "" {
			text = "(фото)"
		}
		// Get largest photo (last in array)
		photo := msg.Photo[len(msg.Photo)-1]
		if t.imageDescriber != nil {
			if desc, err := t.downloadAndDescribe(photo.FileID); err != nil {
				log.Printf("telegram: image describe error: %v", err)
			} else {
				imageDesc = desc
				log.Printf("telegram: image described (%d chars)", len(desc))
			}
		}
	}

	if text == "" && imageDesc == "" {
		return
	}

	chatID := th.ID(msg.Chat.ID)
	sessionID := sessionIDFromChat(msg.Chat.ID)

	// Bot commands (e.g., /compact, /status)
	if strings.HasPrefix(text, "/") && t.commandHandler != nil {
		cmd := strings.SplitN(text, " ", 2)[0]
		cmd = strings.SplitN(cmd, "@", 2)[0] // strip @botname
		if result := t.commandHandler(cmd, sessionID); result != nil {
			t.sendCommandResult(chatID, result)
			return
		}
	}

	userName := msg.From.FirstName
	if msg.From.LastName != "" {
		userName += " " + msg.From.LastName
	}

	// Typing indicator — обновляем каждые 4 секунды пока генерируем ответ
	typingDone := make(chan struct{})
	go func() {
		_ = t.bot.SendChatAction(bgCtx, th.ChatAction(chatID, telego.ChatActionTyping))
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingDone:
				return
			case <-ticker.C:
				_ = t.bot.SendChatAction(bgCtx, th.ChatAction(chatID, telego.ChatActionTyping))
			}
		}
	}()

	incoming := IncomingMessage{
		Text:      text,
		SessionID: sessionID,
		UserID:    userIDFromTG(msg.From.ID),
		UserName:  userName,
		Channel:   "telegram",
		ImageDesc: imageDesc,
	}

	response := t.handler(incoming)
	close(typingDone)

	if response.Text == "" {
		log.Printf("telegram: empty reply for session %s", sessionID)
		return
	}

	log.Printf("telegram: sending reply (%d chars) to session %s", len(response.Text), sessionID)

	// Конвертируем Markdown → HTML для Telegram
	htmlText := markdownToTelegramHTML(response.Text)

	// Разбиваем длинные сообщения (Telegram лимит 4096 символов)
	chunks := splitMessage(htmlText, 4000)
	for _, chunk := range chunks {
		params := th.Message(chatID, chunk)
		params.ParseMode = telego.ModeHTML
		if _, err := t.bot.SendMessage(bgCtx, params); err != nil {
			// Fallback на plain text без разметки
			log.Printf("telegram: HTML send failed, retrying plain: %v", err)
			plainChunks := splitMessage(response.Text, 4000)
			for _, pc := range plainChunks {
				params := th.Message(chatID, pc)
				if _, err := t.bot.SendMessage(bgCtx, params); err != nil {
					log.Printf("telegram: send error: %v", err)
				}
			}
			return
		}
	}
}

// sendCommandResult sends a command result with optional inline keyboard.
func (t *Telegram) sendCommandResult(chatID telego.ChatID, result *CommandResult) {
	if result.Text == "" {
		return
	}
	htmlText := markdownToTelegramHTML(result.Text)
	params := th.Message(chatID, htmlText)
	params.ParseMode = telego.ModeHTML

	// Build inline keyboard if buttons provided
	if len(result.Buttons) > 0 {
		var rows [][]telego.InlineKeyboardButton
		for _, row := range result.Buttons {
			var btns []telego.InlineKeyboardButton
			for _, b := range row {
				btns = append(btns, telego.InlineKeyboardButton{
					Text:         b.Text,
					CallbackData: b.CallbackData,
				})
			}
			rows = append(rows, btns)
		}
		params.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	}

	if _, err := t.bot.SendMessage(bgCtx, params); err != nil {
		// Fallback without HTML
		params.ParseMode = ""
		params.Text = result.Text
		t.bot.SendMessage(bgCtx, params)
	}
}

// handleCallback processes inline keyboard button presses.
func (t *Telegram) handleCallback(cq *telego.CallbackQuery) {
	if t.callbackHandler == nil || cq.Data == "" {
		return
	}

	sessionID := ""
	if cq.Message != nil {
		if msg, ok := cq.Message.(*telego.Message); ok {
			sessionID = sessionIDFromChat(msg.Chat.ID)
		}
	}

	result := t.callbackHandler(cq.Data, sessionID)

	// Answer callback to remove loading indicator
	t.bot.AnswerCallbackQuery(bgCtx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
	})

	if result == nil {
		return
	}

	// Edit the original message with the result
	if cq.Message != nil {
		if msg, ok := cq.Message.(*telego.Message); ok {
			htmlText := markdownToTelegramHTML(result.Text)
			editParams := &telego.EditMessageTextParams{
				ChatID:    th.ID(msg.Chat.ID),
				MessageID: msg.MessageID,
				Text:      htmlText,
				ParseMode: telego.ModeHTML,
			}
			if len(result.Buttons) > 0 {
				var rows [][]telego.InlineKeyboardButton
				for _, row := range result.Buttons {
					var btns []telego.InlineKeyboardButton
					for _, b := range row {
						btns = append(btns, telego.InlineKeyboardButton{
							Text:         b.Text,
							CallbackData: b.CallbackData,
						})
					}
					rows = append(rows, btns)
				}
				editParams.ReplyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
			}
			if _, err := t.bot.EditMessageText(bgCtx, editParams); err != nil {
				log.Printf("telegram: edit message error: %v", err)
			}
			return
		}
	}
}

// SendPhoto sends a photo file to a chat session.
func (t *Telegram) SendPhoto(sessionID, filePath, caption string) error {
	// Extract chat ID from session ID (tg_<chatID>)
	chatIDStr := strings.TrimPrefix(sessionID, "tg_")
	var chatID int64
	for _, c := range chatIDStr {
		if c == '-' {
			continue
		}
		chatID = chatID*10 + int64(c-'0')
	}
	if strings.HasPrefix(chatIDStr, "-") {
		chatID = -chatID
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	params := &telego.SendPhotoParams{
		ChatID: th.ID(chatID),
		Photo:  telego.InputFile{File: file},
	}
	if caption != "" {
		params.Caption = caption
	}

	_, err = t.bot.SendPhoto(bgCtx, params)
	return err
}

// downloadAndDescribe downloads a photo from Telegram and sends it to Grok for description.
func (t *Telegram) downloadAndDescribe(fileID string) (string, error) {
	file, err := t.bot.GetFile(bgCtx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", t.token, file.FilePath)
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}

	mime := "image/jpeg"
	if strings.HasSuffix(file.FilePath, ".png") {
		mime = "image/png"
	}

	return t.imageDescriber.Analyze(data, mime, "")
}

func sessionIDFromChat(chatID int64) string {
	return "tg_" + itoa(chatID)
}

func userIDFromTG(userID int64) string {
	return "tg_user_" + itoa(userID)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// markdownToTelegramHTML converts common Markdown formatting to Telegram HTML.
// Handles: **bold**, *italic*, `code`, ```code blocks```, [text](url)
// Escapes HTML entities in the rest of the text.
func markdownToTelegramHTML(text string) string {
	// First escape HTML entities in source text
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	var result strings.Builder
	lines := strings.Split(text, "\n")

	inCodeBlock := false
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Code blocks (```)
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				result.WriteString("</code></pre>")
				inCodeBlock = false
			} else {
				result.WriteString("<pre><code>")
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			result.WriteString(line)
			continue
		}

		// Process inline formatting
		result.WriteString(convertInlineMarkdown(line))
	}

	if inCodeBlock {
		result.WriteString("</code></pre>")
	}

	return result.String()
}

// convertInlineMarkdown handles **bold**, *italic*, `code` within a single line.
func convertInlineMarkdown(line string) string {
	var result strings.Builder
	runes := []rune(line)
	i := 0

	for i < len(runes) {
		// Inline code: `text`
		if runes[i] == '`' {
			end := indexRune(runes, '`', i+1)
			if end > i {
				result.WriteString("<code>")
				result.WriteString(string(runes[i+1 : end]))
				result.WriteString("</code>")
				i = end + 1
				continue
			}
		}

		// Bold: **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := indexDoubleRune(runes, '*', i+2)
			if end > i {
				result.WriteString("<b>")
				result.WriteString(string(runes[i+2 : end]))
				result.WriteString("</b>")
				i = end + 2
				continue
			}
		}

		// Italic: *text* (single asterisk, not double)
		if runes[i] == '*' && (i+1 >= len(runes) || runes[i+1] != '*') {
			end := indexRune(runes, '*', i+1)
			if end > i {
				result.WriteString("<i>")
				result.WriteString(string(runes[i+1 : end]))
				result.WriteString("</i>")
				i = end + 1
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func indexRune(runes []rune, target rune, start int) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

func indexDoubleRune(runes []rune, target rune, start int) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == target && runes[i+1] == target {
			return i
		}
	}
	return -1
}

func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}
		cut := maxLen
		if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 {
			cut = idx + 1
		}
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	return chunks
}

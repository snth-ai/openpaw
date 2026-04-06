package deliver

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// TelegramClient — канал через Telegram User API (MTProto).
type TelegramClient struct {
	client     *telegram.Client
	handler    Handler
	ctx        context.Context
	cancel     context.CancelFunc
	sessionDir string
}

// TelegramClientConfig — конфиг для user client.
type TelegramClientConfig struct {
	AppID      int
	AppHash    string
	Phone      string
	SessionDir string
}

func NewTelegramClient(cfg TelegramClientConfig, handler Handler) (*TelegramClient, error) {
	if cfg.SessionDir == "" {
		cfg.SessionDir = "./data/tg_session"
	}
	os.MkdirAll(cfg.SessionDir, 0755)

	sessionStorage := &session.FileStorage{
		Path: filepath.Join(cfg.SessionDir, "session.json"),
	}

	ctx, cancel := context.WithCancel(context.Background())

	tc := &TelegramClient{
		handler:    handler,
		ctx:        ctx,
		cancel:     cancel,
		sessionDir: cfg.SessionDir,
	}

	tc.client = telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  telegram.UpdateHandlerFunc(tc.onUpdate),
	})

	return tc, nil
}

func (tc *TelegramClient) Name() string { return "telegram_client" }

func (tc *TelegramClient) Start() error {
	go func() {
		err := tc.client.Run(tc.ctx, func(ctx context.Context) error {
			status, err := tc.client.Auth().Status(ctx)
			if err != nil {
				return fmt.Errorf("auth status: %w", err)
			}
			if !status.Authorized {
				return fmt.Errorf("not authorized — run with --tg-auth first")
			}

			log.Printf("telegram_client: logged in as %s %s (@%s)",
				status.User.FirstName, status.User.LastName, status.User.Username)

			<-ctx.Done()
			return nil
		})
		if err != nil {
			log.Printf("telegram_client: %v", err)
		}
	}()

	log.Printf("telegram_client: starting")
	return nil
}

func (tc *TelegramClient) Stop() {
	tc.cancel()
	log.Printf("telegram_client: stopped")
}

func (tc *TelegramClient) onUpdate(ctx context.Context, u tg.UpdatesClass) error {
	switch updates := u.(type) {
	case *tg.Updates:
		for _, update := range updates.Updates {
			tc.handleUpdate(ctx, update, updates.Users)
		}
	case *tg.UpdatesCombined:
		for _, update := range updates.Updates {
			tc.handleUpdate(ctx, update, updates.Users)
		}
	case *tg.UpdateShort:
		tc.handleUpdate(ctx, updates.Update, nil)
	}
	return nil
}

func (tc *TelegramClient) handleUpdate(ctx context.Context, update tg.UpdateClass, users []tg.UserClass) {
	newMsg, ok := update.(*tg.UpdateNewMessage)
	if !ok {
		return
	}

	msg, ok := newMsg.Message.(*tg.Message)
	if !ok || msg.Out || msg.Message == "" {
		return
	}

	text := strings.TrimSpace(msg.Message)
	if text == "" {
		return
	}

	peer := msg.PeerID
	var sessionID, userID string

	switch p := peer.(type) {
	case *tg.PeerUser:
		sessionID = fmt.Sprintf("tgc_user_%d", p.UserID)
		userID = fmt.Sprintf("tgc_%d", p.UserID)
	case *tg.PeerChat:
		sessionID = fmt.Sprintf("tgc_chat_%d", p.ChatID)
		userID = fmt.Sprintf("tgc_chat_%d", p.ChatID)
	default:
		return
	}

	// Имя отправителя
	userName := ""
	if msg.FromID != nil {
		if fromUser, ok := msg.FromID.(*tg.PeerUser); ok {
			for _, u := range users {
				if user, ok := u.(*tg.User); ok && user.ID == fromUser.UserID {
					userName = user.FirstName
					if user.LastName != "" {
						userName += " " + user.LastName
					}
					break
				}
			}
		}
	}

	incoming := IncomingMessage{
		Text:      text,
		SessionID: sessionID,
		UserID:    userID,
		UserName:  userName,
		Channel:   "telegram_client",
	}

	response := tc.handler(incoming)
	if response.Text == "" {
		return
	}

	api := tc.client.API()
	sender := message.NewSender(api)

	switch p := peer.(type) {
	case *tg.PeerUser:
		sender.To(&tg.InputPeerUser{UserID: p.UserID}).Text(ctx, response.Text)
	case *tg.PeerChat:
		sender.To(&tg.InputPeerChat{ChatID: p.ChatID}).Text(ctx, response.Text)
	}
}

// RunInteractiveAuth — интерактивная авторизация по номеру телефона.
func RunInteractiveAuth(cfg TelegramClientConfig) error {
	if cfg.SessionDir == "" {
		cfg.SessionDir = "./data/tg_session"
	}
	os.MkdirAll(cfg.SessionDir, 0755)

	sessionStorage := &session.FileStorage{
		Path: filepath.Join(cfg.SessionDir, "session.json"),
	}

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: sessionStorage,
	})

	return client.Run(context.Background(), func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if status.Authorized {
			log.Printf("Already authorized as %s", status.User.FirstName)
			return nil
		}

		sentCode, err := client.Auth().SendCode(ctx, cfg.Phone, auth.SendCodeOptions{})
		if err != nil {
			return fmt.Errorf("send code: %w", err)
		}

		var phoneCodeHash string
		switch sc := sentCode.(type) {
		case *tg.AuthSentCode:
			phoneCodeHash = sc.PhoneCodeHash
		default:
			return fmt.Errorf("unexpected sent code type: %T", sentCode)
		}

		fmt.Print("Enter code from Telegram: ")
		var code string
		fmt.Scanln(&code)
		code = strings.TrimSpace(code)

		_, err = client.Auth().SignIn(ctx, cfg.Phone, code, phoneCodeHash)
		if err != nil {
			fmt.Print("Enter 2FA password (if required): ")
			var password string
			fmt.Scanln(&password)
			password = strings.TrimSpace(password)
			if password != "" {
				_, err = client.Auth().Password(ctx, password)
				if err != nil {
					return fmt.Errorf("2FA: %w", err)
				}
			} else {
				return fmt.Errorf("sign in: %w", err)
			}
		}

		log.Printf("Authorization successful!")
		return nil
	})
}

// ParseTelegramClientConfig парсит конфиг из env.
func ParseTelegramClientConfig() *TelegramClientConfig {
	appIDStr := os.Getenv("TELEGRAM_APP_ID")
	appHash := os.Getenv("TELEGRAM_APP_HASH")

	if appIDStr == "" || appHash == "" {
		return nil
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		return nil
	}

	return &TelegramClientConfig{
		AppID:      appID,
		AppHash:    appHash,
		Phone:      os.Getenv("TELEGRAM_CLIENT_PHONE"),
		SessionDir: os.Getenv("TELEGRAM_SESSION_DIR"),
	}
}

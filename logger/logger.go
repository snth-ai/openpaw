package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Level — уровень логирования.
type Level int

const (
	LevelInfo  Level = 0
	LevelDebug Level = 1
)

var (
	currentLevel Level
	useColor     bool
)

// Colors
const (
	reset   = "\033[0m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	gray    = "\033[90m"
	bold    = "\033[1m"
)

func Init() {
	level := os.Getenv("LOG_LEVEL")
	switch strings.ToLower(level) {
	case "debug", "verbose":
		currentLevel = LevelDebug
	default:
		currentLevel = LevelInfo
	}

	// Color if terminal (not piped to file)
	useColor = os.Getenv("NO_COLOR") == "" && isTerminal()

	log.SetFlags(0) // Убираем стандартный prefix, делаем свой
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func timestamp() string {
	return time.Now().Format("15:04:05")
}

// Info — обычное сообщение (всегда видно).
func Info(tag, msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	if useColor {
		fmt.Printf("%s %s%-10s%s %s\n", gray+timestamp()+reset, green, tag, reset, text)
	} else {
		log.Printf("[%s] %s", tag, text)
	}
}

// Debug — подробное сообщение (только при LOG_LEVEL=debug).
func Debug(tag, msg string, args ...any) {
	if currentLevel < LevelDebug {
		return
	}
	text := fmt.Sprintf(msg, args...)
	if useColor {
		fmt.Printf("%s %s%-10s%s %s\n", gray+timestamp()+reset, cyan, tag, reset, gray+text+reset)
	} else {
		log.Printf("[DEBUG][%s] %s", tag, text)
	}
}

// Tool — вызов тула (всегда видно, выделено).
func Tool(name, result string) {
	if len(result) > 200 {
		result = result[:200] + "..."
	}
	if useColor {
		fmt.Printf("%s %s%-10s%s %s%s%s → %s\n", gray+timestamp()+reset, magenta, "tool", reset, bold, name, reset, result)
	} else {
		log.Printf("[tool] %s → %s", name, result)
	}
}

// Emotion — изменение эмоционального состояния.
func Emotion(triggers int, state string) {
	if useColor {
		fmt.Printf("%s %s%-10s%s %d triggers → %s\n", gray+timestamp()+reset, red, "emotion", reset, triggers, state)
	} else {
		log.Printf("[emotion] %d triggers → %s", triggers, state)
	}
}

// Memory — операция с памятью.
func Memory(op, detail string) {
	if useColor {
		fmt.Printf("%s %s%-10s%s %s: %s\n", gray+timestamp()+reset, blue, "memory", reset, op, detail)
	} else {
		log.Printf("[memory] %s: %s", op, detail)
	}
}

// Usage — стоимость запроса.
func Usage(tokens int, cost float64, calls int, durationMs int64) {
	if useColor {
		fmt.Printf("%s %s%-10s%s tokens: %s%d%s cost: %s$%.4f%s calls: %d duration: %dms\n",
			gray+timestamp()+reset, yellow, "usage", reset,
			bold, tokens, reset,
			bold, cost, reset,
			calls, durationMs)
	} else {
		log.Printf("[usage] tokens: %d cost: $%.4f calls: %d duration: %dms", tokens, cost, calls, durationMs)
	}
}

// Channel — входящее/исходящее сообщение.
func Channel(direction, channel, sessionID, text string) {
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	arrow := "→"
	color := green
	if direction == "out" {
		arrow = "←"
		color = cyan
	}
	if useColor {
		fmt.Printf("%s %s%-10s%s %s %s [%s] %s\n",
			gray+timestamp()+reset, color, channel, reset, arrow, sessionID, direction, text)
	} else {
		log.Printf("[%s] %s %s [%s] %s", channel, arrow, sessionID, direction, text)
	}
}

// Error — ошибка (всегда видно).
func Error(tag string, err error) {
	if useColor {
		fmt.Printf("%s %s%-10s%s %s%v%s\n", gray+timestamp()+reset, red+bold, "ERROR", reset, red, err, reset)
	} else {
		log.Printf("[ERROR][%s] %v", tag, err)
	}
}

// Startup — информация при запуске.
func Startup(key, value string) {
	if useColor {
		fmt.Printf("  %s%-20s%s %s\n", green, key, reset, value)
	} else {
		log.Printf("[startup] %s: %s", key, value)
	}
}

// Banner — заголовок при старте.
func Banner() {
	if useColor {
		fmt.Printf("\n%s%s ○ OpenPaw Server %s\n\n", bold, magenta, reset)
	} else {
		log.Println("=== OpenPaw Server ===")
	}
}

// IsDebug возвращает true если debug mode включён.
func IsDebug() bool {
	return currentLevel >= LevelDebug
}

package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// WebFetch — тул для скачивания и извлечения текста с веб-страниц.
type WebFetch struct{}

func (t WebFetch) Name() string { return "web_fetch" }
func (t WebFetch) Description() string {
	return "Fetch a web page and extract its text content. Returns clean text without HTML tags. Use after web_search to read full articles."
}

func (t WebFetch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL of the page to fetch"
			}
		},
		"required": ["url"]
	}`)
}

func (t WebFetch) Execute(args json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", params.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenPaw/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", params.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("fetch %s: status %d", params.URL, resp.StatusCode)
	}

	// Лимит: 1MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	content := htmlToText(string(body))

	// Лимит для контекста
	if len(content) > 30000 {
		content = content[:30000] + "\n... (truncated)"
	}

	if strings.TrimSpace(content) == "" {
		return "(page returned empty content or is JavaScript-rendered)", nil
	}

	return content, nil
}

// htmlToText — простой readability-подобный парсер.
// Убирает теги, скрипты, стили, нормализует пробелы.
func htmlToText(html string) string {
	// Убираем script и style блоки
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = reScript.ReplaceAllString(html, "")

	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = reStyle.ReplaceAllString(html, "")

	// Убираем head
	reHead := regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	html = reHead.ReplaceAllString(html, "")

	// Убираем nav, footer, sidebar
	reNav := regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	html = reNav.ReplaceAllString(html, "")

	reFooter := regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	html = reFooter.ReplaceAllString(html, "")

	// Заменяем блочные теги на переводы строк
	reBlock := regexp.MustCompile(`</(p|div|h[1-6]|li|tr|br|article|section)>`)
	html = reBlock.ReplaceAllString(html, "\n")

	reBr := regexp.MustCompile(`<br\s*/?>`)
	html = reBr.ReplaceAllString(html, "\n")

	// Убираем все оставшиеся теги
	reTags := regexp.MustCompile(`<[^>]+>`)
	html = reTags.ReplaceAllString(html, "")

	// HTML entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Нормализуем пробелы
	reSpaces := regexp.MustCompile(`[ \t]+`)
	html = reSpaces.ReplaceAllString(html, " ")

	// Нормализуем переводы строк (макс 2 подряд)
	reNewlines := regexp.MustCompile(`\n{3,}`)
	html = reNewlines.ReplaceAllString(html, "\n\n")

	// Убираем пробелы в начале строк
	reLineSpaces := regexp.MustCompile(`(?m)^ +`)
	html = reLineSpaces.ReplaceAllString(html, "")

	return strings.TrimSpace(html)
}

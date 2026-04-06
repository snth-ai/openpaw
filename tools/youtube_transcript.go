package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// YouTubeTranscript — тул для получения субтитров через yt-dlp.
type YouTubeTranscript struct{}

func (t YouTubeTranscript) Name() string { return "youtube_transcript" }
func (t YouTubeTranscript) Description() string {
	return "Get transcript/subtitles from a YouTube video. Returns clean text without timecodes. Use to 'watch' a video by reading what was said."
}

func (t YouTubeTranscript) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "YouTube video URL or video ID"
			},
			"lang": {
				"type": "string",
				"description": "Preferred subtitle language code (default: en)"
			}
		},
		"required": ["url"]
	}`)
}

func (t YouTubeTranscript) Execute(args json.RawMessage) (string, error) {
	var params struct {
		URL  string `json:"url"`
		Lang string `json:"lang"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Lang == "" {
		params.Lang = "en"
	}

	videoID := extractVideoID(params.URL)
	if videoID == "" {
		return "", fmt.Errorf("could not extract video ID from %q", params.URL)
	}

	url := "https://www.youtube.com/watch?v=" + videoID

	// Получаем title
	title := getVideoTitle(url)

	// Получаем субтитры через yt-dlp
	text, err := ytdlpTranscript(url, params.Lang)
	if err != nil {
		return "", err
	}

	if len(text) > 40000 {
		text = text[:40000] + "\n... (truncated)"
	}

	if title != "" {
		return fmt.Sprintf("Title: %s\n---\n%s", title, text), nil
	}
	return text, nil
}

func ytdlpTranscript(url, lang string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "yt-sub-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--write-auto-sub",
		"--write-sub",
		"--sub-lang", lang+",en,ru",
		"--skip-download",
		"--convert-subs", "srt",
		"-o", filepath.Join(tmpDir, "%(id)s"),
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp: %s (%w)", string(output), err)
	}

	// Ищем .srt файл
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".srt") {
			data, err := os.ReadFile(filepath.Join(tmpDir, e.Name()))
			if err != nil {
				continue
			}
			text := parseSRT(string(data))
			if text != "" {
				return text, nil
			}
		}
	}

	return "", fmt.Errorf("no subtitles found (yt-dlp output: %s)", string(output))
}

func getVideoTitle(url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp", "--get-title", url)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseSRT убирает таймкоды и индексы из SRT, оставляет чистый текст.
func parseSRT(srt string) string {
	lines := strings.Split(srt, "\n")
	var parts []string
	seen := make(map[string]bool) // дедупликация (auto-subs часто повторяют)

	reTimecode := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}`)
	reIndex := regexp.MustCompile(`^\d+$`)
	reTags := regexp.MustCompile(`<[^>]+>`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || reIndex.MatchString(line) || reTimecode.MatchString(line) {
			continue
		}
		line = reTags.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			seen[line] = true
			parts = append(parts, line)
		}
	}

	return strings.Join(parts, " ")
}

func extractVideoID(input string) string {
	input = strings.TrimSpace(input)

	if len(input) == 11 && !strings.Contains(input, "/") && !strings.Contains(input, ".") {
		return input
	}

	re := regexp.MustCompile(`(?:youtube\.com/watch\?.*v=|youtu\.be/|youtube\.com/embed/|youtube\.com/shorts/)([a-zA-Z0-9_-]{11})`)
	if m := re.FindStringSubmatch(input); len(m) > 1 {
		return m[1]
	}

	return ""
}

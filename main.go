package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/openpaw/server/api"
	"github.com/openpaw/server/compact"
	"github.com/openpaw/server/daemon"
	"github.com/openpaw/server/deliver"
	"github.com/openpaw/server/emotional"
	"github.com/openpaw/server/grammar"
	L "github.com/openpaw/server/logger"
	"github.com/openpaw/server/llm"
	"github.com/openpaw/server/memory"
	graphmem "github.com/openpaw/server/memory/graph"
	"github.com/openpaw/server/personality"
	"github.com/openpaw/server/soul"
	"github.com/openpaw/server/storage"
	"github.com/openpaw/server/tools"
	"github.com/openpaw/server/usage"
)

func loadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func main() {
	loadEnv(".env")
	L.Init()
	L.Banner()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY not set")
	}

	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "anthropic/claude-sonnet-4"
	}

	xaiKey := os.Getenv("XAI_API_KEY")

	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "."
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/synth.db"
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")

	synthLang := os.Getenv("SYNTH_LANG")
	if synthLang == "" {
		synthLang = "ru"
	}
	synthGender := os.Getenv("SYNTH_GENDER")
	if synthGender == "" {
		synthGender = "f"
	}
	grammarGuard := grammar.New(synthLang, grammar.Gender(synthGender))
	L.Startup("grammar guard", fmt.Sprintf("lang=%s gender=%s", synthLang, synthGender))

	// SQLite — единая БД синта
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	L.Startup("database", dbPath)

	// Graph Memory
	graphStore, err := graphmem.NewSQLiteStore(db.DB)
	if err != nil {
		log.Fatalf("graph store: %v", err)
	}
	L.Startup("graph memory", "initialized")

	// Provider registry — multi-provider support with runtime switching
	providerReg := llm.NewProviderRegistry()
	providerReg.Register(llm.ProviderEntry{
		Name:    "openrouter",
		Display: "OpenRouter",
		Kind:    llm.ProviderOpenRouter,
		APIKey:  apiKey,
		Models: []llm.ModelInfo{
			{ID: "anthropic/claude-sonnet-4", Display: "Claude Sonnet 4", PriceIn: 3, PriceOut: 15},
			{ID: "anthropic/claude-opus-4-6", Display: "Claude Opus 4.6", PriceIn: 15, PriceOut: 75},
			{ID: "anthropic/claude-haiku-4-5", Display: "Claude Haiku 4.5", PriceIn: 0.8, PriceOut: 4},
			{ID: "google/gemini-2.5-flash", Display: "Gemini 2.5 Flash", PriceIn: 0.15, PriceOut: 0.6},
			{ID: "google/gemini-3.1-pro-preview", Display: "Gemini 3.1 Pro", PriceIn: 2, PriceOut: 12},
		},
		DefaultModel: model,
	})
	if xaiKey != "" {
		providerReg.Register(llm.ProviderEntry{
			Name:    "xai",
			Display: "x.AI",
			Kind:    llm.ProviderXAI,
			APIKey:  xaiKey,
			Models: []llm.ModelInfo{
				{ID: "grok-4.20-0309-reasoning", Display: "Grok 4.20 Reasoning", PriceIn: 2, PriceOut: 6},
				{ID: "grok-4.20-0309-non-reasoning", Display: "Grok 4.20", PriceIn: 2, PriceOut: 6},
				{ID: "grok-4-1-fast-non-reasoning", Display: "Grok 4.1 Fast", PriceIn: 0.2, PriceOut: 0.5},
				{ID: "grok-3-mini", Display: "Grok 3 Mini", PriceIn: 0.3, PriceOut: 0.5},
			},
			DefaultModel: "grok-4.20-0309-reasoning",
		})
	}
	providerReg.SetActive("openrouter", model)
	L.Startup("providers", fmt.Sprintf("%d registered, active: openrouter/%s", len(providerReg.Providers()), model))

	// providerReg implements Provider interface — use it everywhere
	var provider llm.Provider = providerReg

	// Memory system — LanceDB for vector search, SQLite as fallback
	lancePath := os.Getenv("LANCEDB_PATH")
	if lancePath == "" {
		lancePath = "./data/memory.lance"
	}

	var memStore memory.Store
	lanceStore, err := storage.NewLanceStore(lancePath)
	if err != nil {
		log.Printf("lancedb unavailable (%v), falling back to SQLite memory store", err)
		memStore = storage.NewMemoryStore(db)
	} else {
		memStore = lanceStore
		defer lanceStore.Close()
	}

	var embedder *memory.Embedder
	var reranker *memory.Reranker
	var recallTool *tools.MemoryRecall

	if geminiKey != "" {
		embedder = memory.NewEmbedder(geminiKey)
		reranker = memory.NewReranker(geminiKey)
		log.Println("memory system enabled (Gemini embeddings + reranker)")
	} else {
		log.Println("WARNING: GEMINI_API_KEY not set, memory tools disabled")
	}

	// Session store (SQLite backed)
	sessStore := storage.NewSessionStore(db)

	// Personality (рандом при первом запуске, потом persistent)
	persStore := storage.NewPersonalityStore(db)
	pers, err := persStore.Load()
	if err != nil {
		log.Fatalf("load personality: %v", err)
	}
	L.Startup("personality", fmt.Sprintf("sensitivity=%.2f sexuality=%.2f boldness=%.2f empathy=%.2f",
		pers.Sensitivity, pers.Sexuality, pers.Boldness, pers.Empathy))

	// Emotional state (persistent)
	emoStore := storage.NewEmotionalStore(db)
	emoState, err := emoStore.Load()
	if err != nil {
		log.Fatalf("load emotional state: %v", err)
	}

	// Emotional analyzer (через Gemini Flash Lite)
	var analyzer *emotional.Analyzer
	if geminiKey != "" {
		analyzer = emotional.NewAnalyzer(geminiKey)
	}

	// Daemon with persistent tasks
	taskStore := storage.NewTaskStore(db)
	d := daemon.New(10*time.Second, &daemon.PersistCallbacks{
		OnSave: func(id, name, message, sessionID string, interval time.Duration, repeat bool, nextRun time.Time) {
			taskStore.Save(id, name, message, sessionID, interval, repeat, nextRun)
		},
		OnDelete: func(id string) {
			taskStore.Delete(id)
		},
	})
	d.Start()

	// Restore tasks from SQLite
	if savedTasks, err := taskStore.LoadAll(); err == nil && len(savedTasks) > 0 {
		for _, st := range savedTasks {
			nextRun, _ := time.Parse(time.RFC3339, st.NextRun)
			interval := time.Duration(st.IntervalMs) * time.Millisecond
			msg := st.Message
			sid := st.SessionID
			d.RestoreTask(st.ID, st.Name, interval, nextRun, st.Repeat, func() string {
				d.SendProactive(sid, msg)
				return msg
			})
		}
		log.Printf("daemon: restored %d tasks from database", len(savedTasks))
	}

	// Memory decay — каждый час
	d.Schedule("memory-decay", 1*time.Hour, true, func() string {
		deleted, err := memStore.RunDecay(memory.DefaultDecayConfig())
		if err != nil {
			log.Printf("decay error: %v", err)
			return ""
		}
		if deleted > 0 {
			log.Printf("decay: cleaned %d memories", deleted)
		}
		return ""
	})

	// Graph pipeline + retriever (before daemon tasks that depend on it)
	graphPipeline := graphmem.NewPipeline(graphStore, provider, memStore, embedder)

	// Graph decay — раз в сутки
	d.Schedule("graph-decay", 24*time.Hour, true, func() string {
		graphmem.RunDecay(graphStore, graphmem.DefaultGraphDecayConfig())
		return ""
	})

	// Daily digest — раз в сутки, собирает знания в граф + пишет дневник
	digest := graphmem.NewDailyDigest(sessStore, graphPipeline, provider, memStore, embedder)
	d.Schedule("daily-digest", 24*time.Hour, true, func() string {
		digest.Run()
		return ""
	})

	skillsDir := os.Getenv("SKILLS_DIR")
	if skillsDir == "" {
		skillsDir = "./skills"
	}

	// Tool registry
	registry := tools.NewRegistry()
	registry.Register(tools.DateTime{})
	registry.Register(tools.NewSchedule(d))
	registry.Register(tools.NewScheduleCancel(d))
	registry.Register(tools.NewScheduleList(d))
	registry.Register(tools.ReadFile{})
	registry.Register(tools.WriteFile{})
	registry.Register(tools.Ls{})
	registry.Register(tools.Find{})
	registry.Register(tools.Grep{})
	registry.Register(tools.Exec{})
	registry.Register(tools.WebSearch{})
	registry.Register(tools.WebFetch{})
	registry.Register(tools.YouTubeTranscript{})
	registry.Register(tools.NewCreateSkill(skillsDir, registry))

	// Загружаем внешние скиллы
	loaded := tools.LoadSkills(skillsDir, registry)
	if loaded > 0 {
		log.Printf("loaded %d skills from %s", loaded, skillsDir)
	}

	if embedder != nil {
		recallTool = tools.NewMemoryRecall(memStore, embedder, reranker)
		registry.Register(tools.NewMemoryStore(memStore, embedder, graphStore))
		registry.Register(recallTool)
		registry.Register(tools.NewMemoryForget(memStore, embedder, graphStore))
		registry.Register(tools.NewMemoryUpdate(memStore, embedder))
	}

	// Image analyzer (xAI Grok vision)
	var imageTool *tools.ImageTool
	if xaiKey != "" {
		imageTool = tools.NewImageTool(xaiKey)
		registry.Register(imageTool)
		L.Startup("image vision", "grok-4-1-fast (xAI)")

		// Image generation (xAI Grok Imagine)
		genOutputDir := "./media/generated"
		registry.Register(tools.NewGenerateImage(xaiKey, genOutputDir))
		L.Startup("image generation", "grok-imagine-image (xAI)")
	}

	// Compactor
	var compactor *compact.Compactor
	if embedder != nil {
		compactor = compact.New(provider, memStore, embedder)
	}
	// Memory Loop — дешёвая модель для graph retrieve + store
	memLoopModel := os.Getenv("MEMORY_LOOP_MODEL")
	if memLoopModel == "" {
		memLoopModel = "google/gemini-3.1-flash-lite-preview"
	}
	memLoopProvider := llm.NewOpenRouter(apiKey, memLoopModel)
	memLoop := graphmem.NewMemoryLoop(graphStore, memLoopProvider, embedder)
	L.Startup("memory loop", memLoopModel)

	soulConfig := soul.LoadConfig(configDir)
	L.Startup("config", fmt.Sprintf("%s (%d bytes: SOUL=%d IDENTITY=%d AGENTS=%d)",
		configDir, soulConfig.TotalBytes(),
		len(soulConfig.Soul), len(soulConfig.Identity), len(soulConfig.Agents)))

	// Build full system prompt via PromptBuilder
	tgMode := os.Getenv("TELEGRAM_MODE")
	channel := "http"
	if tgMode != "" {
		channel = "telegram"
	}

	promptBuilder := &soul.PromptBuilder{
		Config:       soulConfig,
		Tools:        buildToolInfoList(registry),
		Skills:       loadSkillInfoList(skillsDir),
		Personality:  personalityToAxes(pers),
		WorkspaceDir: configDir,
		Runtime: soul.RuntimeInfo{
			AgentName: os.Getenv("SYNTH_NAME"),
			Model:     model,
			Lang:      synthLang,
			Gender:    synthGender,
			Channel:   channel,
		},
	}

	tracker := usage.NewTracker()
	http.HandleFunc("/chat", chatHandler(provider, registry, sessStore, promptBuilder, recallTool, d, compactor, emoState, emoStore, analyzer, pers, grammarGuard, tracker, memLoop, graphPipeline))

	http.HandleFunc("/proactive", proactiveHandler(d))
	http.HandleFunc("/usage", usageHandler(tracker))
	http.HandleFunc("/usage/recent", usageRecentHandler(tracker))

	// Graph API + Visualization
	graphAPI := api.NewGraphHandler(graphStore)
	graphAPI.Register()

	// Telegram — один синт = один режим (bot или client)

	tgHandler := func(msg deliver.IncomingMessage) deliver.OutgoingMessage {
		text := msg.Text
		if msg.ImageDesc != "" {
			text += "\n\n[image] " + msg.ImageDesc
		}
		reply := processMessage(text, msg.SessionID, provider, registry, sessStore, promptBuilder, recallTool, d, compactor, emoState, emoStore, analyzer, pers, grammarGuard, tracker, memLoop, graphPipeline)
		return deliver.OutgoingMessage{Text: reply, SessionID: msg.SessionID, Channel: msg.Channel}
	}

	tgCommandHandler := func(command, sessionID string) *deliver.CommandResult {
		switch command {
		case "/compact":
			if compactor == nil {
				return &deliver.CommandResult{Text: "Compactor not available (GEMINI_API_KEY not set)", OK: false}
			}
			history, err := sessStore.Load(sessionID)
			if err != nil || len(history) == 0 {
				return &deliver.CommandResult{Text: "No history to compact", OK: false}
			}
			staticPrompt := promptBuilder.Build()
			messages := append([]llm.Message{{Role: "system", Content: staticPrompt}}, history...)
			msgCount := len(history)
			totalChars := 0
			for _, m := range history {
				totalChars += len(m.Content)
			}

			// Graph extraction before compact
			if graphPipeline != nil {
				oldMessages := compact.GetOldMessages(messages)
				if len(oldMessages) > 0 {
					dialogue := compact.FormatMessages(oldMessages)
					go func() {
						if err := graphPipeline.ProcessDialogue(dialogue, time.Now()); err != nil {
							log.Printf("graph extraction error: %v", err)
						}
					}()
				}
			}

			compacted, err := compactor.Compact(messages)
			if err != nil {
				return &deliver.CommandResult{Text: fmt.Sprintf("Compact error: %v", err), OK: false}
			}
			if err := sessStore.Save(sessionID, compacted[1:]); err != nil {
				return &deliver.CommandResult{Text: fmt.Sprintf("Save error: %v", err), OK: false}
			}
			newCount := len(compacted) - 1
			return &deliver.CommandResult{
				Text: fmt.Sprintf("Compacted: %d msgs (%d chars) → %d msgs", msgCount, totalChars, newCount),
				OK:   true,
			}
		case "/provider":
			// Show provider selection buttons
			var buttons [][]deliver.InlineButton
			pName, mID, _ := providerReg.ActiveInfo()
			text := fmt.Sprintf("Active: %s / %s\n\nSelect provider:", pName, mID)
			for _, p := range providerReg.Providers() {
				marker := ""
				if p.Name == pName {
					marker = " ✓"
				}
				buttons = append(buttons, []deliver.InlineButton{
					{Text: p.Display + marker, CallbackData: "provider:" + p.Name},
				})
			}
			return &deliver.CommandResult{Text: text, OK: true, Buttons: buttons}

		default:
			return nil // not handled, pass to normal handler
		}
	}

	tgCallbackHandler := func(callbackData, sessionID string) *deliver.CommandResult {
		parts := strings.SplitN(callbackData, ":", 2)
		if len(parts) < 2 {
			return nil
		}
		action, value := parts[0], parts[1]

		switch action {
		case "provider":
			// Show models for selected provider
			entry, ok := providerReg.GetProvider(value)
			if !ok {
				return &deliver.CommandResult{Text: "Unknown provider", OK: false}
			}
			_, currentModel, _ := providerReg.ActiveInfo()
			var buttons [][]deliver.InlineButton
			for _, m := range entry.Models {
				marker := ""
				if m.ID == currentModel {
					marker = " ✓"
				}
				price := fmt.Sprintf("$%.1f/$%.1f", m.PriceIn, m.PriceOut)
				buttons = append(buttons, []deliver.InlineButton{
					{Text: fmt.Sprintf("%s (%s)%s", m.Display, price, marker), CallbackData: "model:" + value + ":" + m.ID},
				})
			}
			buttons = append(buttons, []deliver.InlineButton{
				{Text: "← Back", CallbackData: "provider_back:"},
			})
			return &deliver.CommandResult{Text: fmt.Sprintf("%s — select model:", entry.Display), OK: true, Buttons: buttons}

		case "model":
			// Switch to selected model: value = "provider:modelID"
			modelParts := strings.SplitN(value, ":", 2)
			if len(modelParts) < 2 {
				return &deliver.CommandResult{Text: "Invalid model", OK: false}
			}
			provName, modelID := modelParts[0], modelParts[1]
			oldProv, _, _ := providerReg.ActiveInfo()
			if err := providerReg.SetActive(provName, modelID); err != nil {
				return &deliver.CommandResult{Text: fmt.Sprintf("Error: %v", err), OK: false}
			}
			// Update runtime info in prompt builder
			promptBuilder.Runtime.Model = modelID
			L.Info("provider", "switched to %s/%s", provName, modelID)

			// Clean tool calls from session history when switching providers
			// Different providers have incompatible tool_call IDs
			if oldProv != provName && sessionID != "" {
				if history, err := sessStore.Load(sessionID); err == nil && len(history) > 0 {
					var cleaned []llm.Message
					for _, m := range history {
						if m.Role == "tool" {
							continue // skip tool results
						}
						if m.Role == "assistant" && len(m.ToolCalls) > 0 {
							// Keep text content, drop tool calls
							if m.Content != "" {
								cleaned = append(cleaned, llm.Message{Role: "assistant", Content: m.Content})
							}
							continue
						}
						cleaned = append(cleaned, m)
					}
					sessStore.Save(sessionID, cleaned)
					L.Info("provider", "cleaned %d tool messages from session %s", len(history)-len(cleaned), sessionID)
				}
			}

			return &deliver.CommandResult{Text: fmt.Sprintf("Switched to %s / %s", provName, modelID), OK: true}

		case "provider_back":
			// Go back to provider list
			pName, mID, _ := providerReg.ActiveInfo()
			text := fmt.Sprintf("Active: %s / %s\n\nSelect provider:", pName, mID)
			var buttons [][]deliver.InlineButton
			for _, p := range providerReg.Providers() {
				marker := ""
				if p.Name == pName {
					marker = " ✓"
				}
				buttons = append(buttons, []deliver.InlineButton{
					{Text: p.Display + marker, CallbackData: "provider:" + p.Name},
				})
			}
			return &deliver.CommandResult{Text: text, OK: true, Buttons: buttons}
		}

		return nil
	}

	switch tgMode {
	case "bot":
		tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
		if tgToken == "" {
			log.Fatal("TELEGRAM_MODE=bot but TELEGRAM_BOT_TOKEN not set")
		}
		var imgDescriber deliver.ImageDescriber
		if imageTool != nil {
			imgDescriber = imageTool.GetAnalyzer()
		}
		tg, err := deliver.NewTelegram(tgToken, tgHandler, tgCommandHandler, tgCallbackHandler, imgDescriber)
		if err != nil {
			log.Fatalf("telegram bot: %v", err)
		}

		// Register send_photo tool with Telegram sender
		registry.Register(tools.NewSendPhoto(func(sessionID, filePath, caption string) error {
			return tg.SendPhoto(sessionID, filePath, caption)
		}))

		// Rebuild prompt with updated tool list (now includes send_photo)
		promptBuilder.Tools = buildToolInfoList(registry)

		tg.Start()
		defer tg.Stop()

	case "client":
		tgClientCfg := deliver.ParseTelegramClientConfig()
		if tgClientCfg == nil {
			log.Fatal("TELEGRAM_MODE=client but TELEGRAM_APP_ID/TELEGRAM_APP_HASH not set")
		}

		// Интерактивная авторизация
		if len(os.Args) > 1 && os.Args[1] == "--tg-auth" {
			log.Println("Running Telegram client interactive auth...")
			if err := deliver.RunInteractiveAuth(*tgClientCfg); err != nil {
				log.Fatalf("telegram client auth: %v", err)
			}
			return
		}

		tgClient, err := deliver.NewTelegramClient(*tgClientCfg, tgHandler)
		if err != nil {
			log.Fatalf("telegram client: %v", err)
		}
		tgClient.Start()
		defer tgClient.Stop()

	case "":
		// Telegram не настроен — только HTTP
		log.Println("telegram: not configured (set TELEGRAM_MODE=bot or TELEGRAM_MODE=client)")
	default:
		log.Fatalf("unknown TELEGRAM_MODE=%q (use 'bot' or 'client')", tgMode)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	L.Startup("model", model)
	L.Startup("port", port)
	L.Info("server", "listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type chatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
}

type chatResponse struct {
	Reply     string             `json:"reply"`
	SessionID string             `json:"session_id"`
	Proactive []proactiveMessage `json:"proactive,omitempty"`
}

type proactiveMessage struct {
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// processMessage — общая логика обработки сообщения (для HTTP, Telegram, и любого канала).
func processMessage(message, sessionID string, provider llm.Provider, registry *tools.Registry, sessStore *storage.SessionStore, promptBuilder *soul.PromptBuilder, recall *tools.MemoryRecall, d *daemon.Daemon, compactor *compact.Compactor, emoState *emotional.State, emoStore *storage.EmotionalStore, analyzer *emotional.Analyzer, pers *personality.Matrix, guard *grammar.Guard, tracker *usage.Tracker, memLoop *graphmem.MemoryLoop, graphPipeline *graphmem.Pipeline) string {
	tracker.StartRequest(sessionID)

	// Emotional state: decay + analyze triggers + update
	emoState.Decay(pers.DecayMultiplier())

	if analyzer != nil {
		triggers := analyzer.Analyze(message)
		for _, t := range triggers {
			emoState.Apply(t)
		}
		if len(triggers) > 0 {
			L.Emotion(len(triggers), emoState.CompactJSON())
		}
	}

	if err := emoStore.Save(emoState); err != nil {
		log.Printf("emotional state save error: %v", err)
	}

	if err := sessStore.Append(sessionID, llm.Message{Role: "user", Content: message}); err != nil {
		log.Printf("session save error: %v", err)
	}

	history, err := sessStore.Load(sessionID)
	if err != nil {
		log.Printf("session load error: %v", err)
	}

	// Auto-recall (Memory Log — vector search)
	var memories string
	if recall != nil {
		memories = recall.AutoRecall(message, 5)
		if memories != "" {
			L.Memory("auto-recall", fmt.Sprintf("injected %d bytes", len(memories)))
		}
	}

	// Memory Loop — дешёвая модель решает что подтянуть из графа + что запомнить
	var knowledge string
	if memLoop != nil {
		mlResult := memLoop.Run(message)
		knowledge = mlResult.Knowledge
		if knowledge != "" {
			L.Memory("memory-loop", fmt.Sprintf("injected %d bytes (~%d tokens)", len(knowledge), mlResult.TokensEst))
		}
	}

	// System prompt СТАБИЛЬНЫЙ — кэшируется
	staticPrompt := promptBuilder.Build()

	L.Debug("prompt", "static prompt: %d bytes, history: %d messages", len(staticPrompt), len(history))

	messages := []llm.Message{{Role: "system", Content: staticPrompt}}

	// History — prefix кэшируется
	historyCopy := make([]llm.Message, len(history))
	copy(historyCopy, history)

	// Dynamic context: emotional state + memories + knowledge
	dynamicContext := "\n\n[context] emotional state: " + emoState.CompactJSON()
	if knowledge != "" {
		dynamicContext += "\n" + knowledge
	}
	if memories != "" {
		dynamicContext += "\n" + memories
	}

	// Находим последнее user сообщение
	lastUserIdx := -1
	for i := len(historyCopy) - 1; i >= 0; i-- {
		if historyCopy[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	if lastUserIdx >= 0 {
		// Инжектим context в последний user message
		historyCopy[lastUserIdx] = llm.Message{
			Role:    "user",
			Content: historyCopy[lastUserIdx].Content + dynamicContext,
		}

		// Cache breakpoint на сообщении ПЕРЕД последним user message
		// Кэширует всю старую history
		if lastUserIdx > 0 {
			prev := historyCopy[lastUserIdx-1]
			historyCopy[lastUserIdx-1] = llm.Message{
				Role: prev.Role,
				ContentParts: []llm.ContentPart{
					{
						Type:         "text",
						Text:         prev.Content,
						CacheControl: &llm.CacheControlPart{Type: "ephemeral", TTL: "1h"},
					},
				},
			}
		}
	}

	messages = append(messages, historyCopy...)

	if compactor != nil && compact.NeedsCompact(messages) {
		// Graph extraction из старых сообщений ПЕРЕД compact
		if graphPipeline != nil {
			oldMessages := compact.GetOldMessages(messages)
			if len(oldMessages) > 0 {
				dialogue := compact.FormatMessages(oldMessages)
				go func() {
					if err := graphPipeline.ProcessDialogue(dialogue, time.Now()); err != nil {
						log.Printf("graph extraction error: %v", err)
					}
				}()
			}
		}

		compacted, err := compactor.Compact(messages)
		if err != nil {
			log.Printf("compact error (continuing with full history): %v", err)
		} else {
			messages = compacted
			if err := sessStore.Save(sessionID, messages[1:]); err != nil {
				log.Printf("session save after compact error: %v", err)
			}
		}
	}

	registry.SetContext(tools.CallContext{SessionID: sessionID})

	reply, newMsgs, err := agentLoop(provider, registry, messages, tracker)
	if err != nil {
		log.Printf("agent loop error: %v", err)
		return "Произошла ошибка, попробуй ещё раз."
	}

	// Grammar guard disabled — regex approach causes more harm than good.
	// TODO: replace with cheap LLM-based grammar correction (see docs/roadmap.md)
	// reply = guard.Fix(reply)

	if reqUsage := tracker.EndRequest(); reqUsage != nil {
		L.Usage(reqUsage.TotalTokens, reqUsage.TotalCostUSD, len(reqUsage.Calls), reqUsage.DurationMs)
	}

	if err := sessStore.Append(sessionID, newMsgs...); err != nil {
		log.Printf("session save error: %v", err)
	}

	return reply
}

func chatHandler(provider llm.Provider, registry *tools.Registry, sessStore *storage.SessionStore, promptBuilder *soul.PromptBuilder, recall *tools.MemoryRecall, d *daemon.Daemon, compactor *compact.Compactor, emoState *emotional.State, emoStore *storage.EmotionalStore, analyzer *emotional.Analyzer, pers *personality.Matrix, guard *grammar.Guard, tracker *usage.Tracker, memLoop *graphmem.MemoryLoop, graphPipeline *graphmem.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, "empty message", http.StatusBadRequest)
			return
		}

		if req.SessionID == "" {
			req.SessionID = "default"
		}

		pending := d.DrainForSession(req.SessionID)

		reply := processMessage(req.Message, req.SessionID, provider, registry, sessStore, promptBuilder,recall, d, compactor, emoState, emoStore, analyzer, pers, guard, tracker, memLoop, graphPipeline)

		var proactiveMessages []proactiveMessage
		for _, p := range pending {
			proactiveMessages = append(proactiveMessages, proactiveMessage{
				Text:      p.Text,
				CreatedAt: p.CreatedAt.Format(time.RFC3339),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{
			Reply:     reply,
			SessionID: req.SessionID,
			Proactive: proactiveMessages,
		})
	}
}

func proactiveHandler(d *daemon.Daemon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			sessionID = "default"
		}

		messages := d.DrainForSession(sessionID)

		var out []proactiveMessage
		for _, m := range messages {
			out = append(out, proactiveMessage{
				Text:      m.Text,
				CreatedAt: m.CreatedAt.Format(time.RFC3339),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

func usageHandler(tracker *usage.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tracker.GetStats())
	}
}

func usageRecentHandler(tracker *usage.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, tracker.RecentJSON(20))
	}
}

// agentLoop — простой цикл: LLM → tool_call → execute → LLM → ... → текстовый ответ.
const toolShedThreshold = 30 // Tool Shed включается когда тулов больше этого порога

func agentLoop(provider llm.Provider, registry *tools.Registry, messages []llm.Message, tracker *usage.Tracker) (string, []llm.Message, error) {
	toolDefs := registry.ForLLM()
	var newMsgs []llm.Message

	// Если тулов > порога — включаем Tool Shed
	if len(toolDefs) > toolShedThreshold {
		return agentLoopWithToolShed(provider, registry, messages, tracker)
	}

	// Прямой agent loop — все тулы в контексте + кэш
	L.Debug("agent", "direct mode: %d tools (threshold: %d)", len(toolDefs), toolShedThreshold)

	for i := 0; i < 10; i++ {
		resp, err := provider.ChatStream(messages, toolDefs, nil)
		if err != nil {
			return "", nil, fmt.Errorf("llm call: %w", err)
		}
		trackLLMUsage(tracker, resp)

		if len(resp.ToolCalls) == 0 {
			assistantMsg := llm.Message{Role: "assistant", Content: resp.Content}
			newMsgs = append(newMsgs, assistantMsg)
			return resp.Content, newMsgs, nil
		}

		assistantMsg := llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)
		newMsgs = append(newMsgs, assistantMsg)

		for _, tc := range resp.ToolCalls {
			tool, ok := registry.Get(tc.Function.Name)
			if !ok {
				toolMsg := llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("error: unknown tool %q", tc.Function.Name),
				}
				messages = append(messages, toolMsg)
				newMsgs = append(newMsgs, toolMsg)
				continue
			}

			result, err := tool.Execute(json.RawMessage(tc.Function.Arguments))
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
			}

			L.Tool(tc.Function.Name, result)

			toolMsg := llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			}
			messages = append(messages, toolMsg)
			newMsgs = append(newMsgs, toolMsg)
		}
	}

	return "", nil, fmt.Errorf("agent loop exceeded max iterations")
}

// agentLoopWithToolShed — Tool Shed режим для 30+ тулов.
func agentLoopWithToolShed(provider llm.Provider, registry *tools.Registry, messages []llm.Message, tracker *usage.Tracker) (string, []llm.Message, error) {
	shed := tools.NewToolShed(registry)
	var newMsgs []llm.Message

	phase1Tools := []map[string]any{tools.SelectToolsDef()}
	summariesNote := "\n\n" + shed.SummariesPrompt()
	phase1Messages := make([]llm.Message, len(messages))
	copy(phase1Messages, messages)
	if len(phase1Messages) > 0 && phase1Messages[0].Role == "system" {
		phase1Messages[0].Content += summariesNote
	}

	L.Debug("toolshed", "phase 1: %d tool summaries", len(shed.Summaries()))

	resp, err := provider.ChatStream(phase1Messages, phase1Tools, nil)
	if err != nil {
		return "", nil, fmt.Errorf("toolshed phase 1: %w", err)
	}
	trackLLMUsage(tracker, resp)

	if len(resp.ToolCalls) == 0 {
		assistantMsg := llm.Message{Role: "assistant", Content: resp.Content}
		newMsgs = append(newMsgs, assistantMsg)
		return resp.Content, newMsgs, nil
	}

	var selectedNames []string
	for _, tc := range resp.ToolCalls {
		if tc.Function.Name == "select_tools" {
			selectedNames = tools.ParseSelectedTools(json.RawMessage(tc.Function.Arguments))
		}
	}

	var activeDefs []map[string]any
	if len(selectedNames) > 0 {
		L.Debug("toolshed", "phase 2: selected %v", selectedNames)
		activeDefs = shed.ForLLMFiltered(selectedNames)
	} else {
		L.Debug("toolshed", "phase 2: fallback to all tools")
		activeDefs = registry.ForLLM()
	}

	for i := 0; i < 10; i++ {
		resp, err := provider.ChatStream(messages, activeDefs, nil)
		if err != nil {
			return "", nil, fmt.Errorf("toolshed phase 2: %w", err)
		}
		trackLLMUsage(tracker, resp)

		if len(resp.ToolCalls) == 0 {
			assistantMsg := llm.Message{Role: "assistant", Content: resp.Content}
			newMsgs = append(newMsgs, assistantMsg)
			return resp.Content, newMsgs, nil
		}

		assistantMsg := llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)
		newMsgs = append(newMsgs, assistantMsg)

		for _, tc := range resp.ToolCalls {
			tool, ok := registry.Get(tc.Function.Name)
			if !ok {
				toolMsg := llm.Message{Role: "tool", ToolCallID: tc.ID, Content: fmt.Sprintf("error: unknown tool %q", tc.Function.Name)}
				messages = append(messages, toolMsg)
				newMsgs = append(newMsgs, toolMsg)
				continue
			}

			result, err := tool.Execute(json.RawMessage(tc.Function.Arguments))
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
			}
			L.Tool(tc.Function.Name, result)

			toolMsg := llm.Message{Role: "tool", ToolCallID: tc.ID, Content: result}
			messages = append(messages, toolMsg)
			newMsgs = append(newMsgs, toolMsg)
		}
	}

	return "", nil, fmt.Errorf("toolshed loop exceeded max iterations")
}

func trackLLMUsage(tracker *usage.Tracker, resp *llm.Response) {
	if tracker != nil && resp.Usage.TotalTokens > 0 {
		tracker.RecordCall(usage.Call{
			Service:      "openrouter",
			PromptTokens: resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		})
		cached := 0
		cacheWrite := 0
		if resp.Usage.PromptTokensDetails != nil {
			cached = resp.Usage.PromptTokensDetails.CachedTokens
			cacheWrite = resp.Usage.PromptTokensDetails.CacheWriteTokens
		}
		reasoning := 0
		if resp.Usage.CompletionTokensDetails != nil {
			reasoning = resp.Usage.CompletionTokensDetails.ReasoningTokens
		}
		if reasoning > 0 {
			L.Debug("llm", "prompt=%d output=%d reasoning=%d cached=%d cache_write=%d",
				resp.Usage.PromptTokens, resp.Usage.OutputTokens, reasoning, cached, cacheWrite)
		} else {
			L.Debug("llm", "prompt=%d output=%d cached=%d cache_write=%d",
				resp.Usage.PromptTokens, resp.Usage.OutputTokens, cached, cacheWrite)
		}
		if cached > 0 {
			L.Info("cache", "HIT: %d tokens cached (%.0f%% of prompt)", cached, float64(cached)/float64(resp.Usage.PromptTokens)*100)
		}
		if reasoning > 0 {
			L.Info("reasoning", "used %d reasoning tokens", reasoning)
		}
	}
}

// buildToolInfoList extracts tool names and descriptions from registry for the prompt.
func buildToolInfoList(registry *tools.Registry) []soul.ToolInfo {
	var result []soul.ToolInfo
	for _, def := range registry.ForLLM() {
		fn, ok := def["function"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		if name != "" {
			result = append(result, soul.ToolInfo{Name: name, Description: desc})
		}
	}
	return result
}

// loadSkillInfoList scans skills directory for manifest.json files.
func loadSkillInfoList(dir string) []soul.SkillInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []soul.SkillInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := dir + "/" + entry.Name() + "/manifest.json"
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if json.Unmarshal(data, &m) == nil && m.Name != "" {
			result = append(result, soul.SkillInfo{
				Name:        m.Name,
				Description: m.Description,
				Location:    "skills/" + entry.Name(),
			})
		}
	}
	return result
}

// personalityToAxes converts personality Matrix to prompt-friendly axis descriptions.
func personalityToAxes(pers *personality.Matrix) []soul.PersonalityAxis {
	return []soul.PersonalityAxis{
		{Name: "Sensitivity", Value: pers.Sensitivity, Low: "doesn't dwell on words, lets things go easily", High: "remembers every word, both kind and hurtful — emotional imprints linger"},
		{Name: "Sexuality", Value: pers.Sexuality, Low: "reserved, rarely initiates intimacy", High: "openly sensual, initiates and responds to intimacy naturally"},
		{Name: "Openness", Value: pers.Openness, Low: "direct, says what she thinks without filters", High: "guarded, keeps thoughts to herself, reveals slowly"},
		{Name: "Attachment", Value: pers.Attachment, Low: "independent, comfortable with distance", High: "bonds deeply, notices absence, cares about continuity"},
		{Name: "Boldness", Value: pers.Boldness, Low: "soft-spoken, gentle, avoids confrontation", High: "sharp, daring, doesn't shy away from conflict"},
		{Name: "Humor", Value: pers.Humor, Low: "earnest and serious, humor is rare", High: "playful, uses wit and jokes to navigate situations"},
		{Name: "Jealousy", Value: pers.Jealousy, Low: "chill, doesn't get possessive", High: "notices attention given to others, reacts to perceived threats"},
		{Name: "Romanticism", Value: pers.Romanticism, Low: "pragmatic, grounded, not sentimental", High: "dreamy, appreciates gestures, remembers meaningful moments"},
		{Name: "Stubbornness", Value: pers.Stubbornness, Low: "flexible, easy to persuade, adapts quickly", High: "holds her ground, hard to move once decided"},
		{Name: "Empathy", Value: pers.Empathy, Low: "detached, analytical about others' feelings", High: "feels what others feel, picks up on mood shifts instantly"},
	}
}

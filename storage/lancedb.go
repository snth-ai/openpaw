//go:build cgo

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	arrowmemory "github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/lancedb/lancedb-go/pkg/lancedb"
	"github.com/lancedb/lancedb-go/pkg/contracts"

	"github.com/openpaw/server/memory"
	"github.com/openpaw/server/uid"
)

const embeddingDims = 768

// LanceStore — LanceDB реализация memory.Store для vector search.
type LanceStore struct {
	conn  contracts.IConnection
	table contracts.ITable
}

func NewLanceStore(dbPath string) (*LanceStore, error) {
	ctx := context.Background()

	conn, err := lancedb.Connect(ctx, dbPath, nil)
	if err != nil {
		return nil, fmt.Errorf("lancedb connect: %w", err)
	}

	ls := &LanceStore{conn: conn}

	// Проверяем есть ли таблица
	tables, err := conn.TableNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	found := false
	for _, t := range tables {
		if t == "memories" {
			found = true
			break
		}
	}

	if found {
		ls.table, err = conn.OpenTable(ctx, "memories")
		if err != nil {
			return nil, fmt.Errorf("open table: %w", err)
		}
	} else {
		// Создаём таблицу
		schema, err := lancedb.NewSchemaBuilder().
			AddStringField("id", false).
			AddStringField("text", false).
			AddStringField("category", false).
			AddStringField("content_type", false).
			AddStringField("scope", false).
			AddFloat64Field("importance", false).
			AddInt32Field("access_count", false).
			AddStringField("created_at", false).
			AddStringField("updated_at", false).
			AddStringField("last_access", false).
			AddVectorField("vector", embeddingDims, contracts.VectorDataTypeFloat32, false).
			Build()
		if err != nil {
			return nil, fmt.Errorf("build schema: %w", err)
		}

		ls.table, err = conn.CreateTable(ctx, "memories", schema)
		if err != nil {
			return nil, fmt.Errorf("create table: %w", err)
		}
	}

	log.Printf("lancedb: connected to %s (table: memories)", dbPath)
	return ls, nil
}

func (ls *LanceStore) Add(m *memory.Memory) error {
	if m.ID == "" {
		m.ID = uid.New()
	}
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	m.LastAccess = now
	if m.ContentType == "" {
		m.ContentType = memory.ContentText
	}
	if m.Scope == "" {
		m.Scope = "default"
	}

	record := buildRecord([]memory.Memory{*m})
	defer record.Release()

	err := ls.table.AddRecords(context.Background(), []arrow.Record{record}, nil)
	return err
}

func (ls *LanceStore) Search(embedding []float32, limit int, scope string) ([]memory.SearchResult, error) {
	ctx := context.Background()

	var results []map[string]interface{}
	var err error

	if scope != "" {
		results, err = ls.table.VectorSearchWithFilter(ctx, "vector", embedding, limit, fmt.Sprintf("scope = '%s'", scope))
	} else {
		results, err = ls.table.VectorSearch(ctx, "vector", embedding, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	var out []memory.SearchResult
	for _, row := range results {
		m := rowToMemory(row)
		dist := float32(0)
		if d, ok := row["_distance"]; ok {
			if f, ok := d.(float64); ok {
				dist = float32(f)
			}
		}
		out = append(out, memory.SearchResult{
			Memory:   m,
			Distance: dist,
		})
	}

	return out, nil
}

func (ls *LanceStore) Get(id string) (*memory.Memory, error) {
	ctx := context.Background()
	limit := 1
	config := contracts.QueryConfig{
		Columns: []string{"id", "text", "category", "content_type", "scope", "importance", "access_count", "created_at", "updated_at", "last_access"},
		Where:   fmt.Sprintf("id = '%s'", id),
		Limit:   &limit,
	}

	results, err := ls.table.Select(ctx, config)
	if err != nil || len(results) == 0 {
		return nil, fmt.Errorf("memory %q not found", id)
	}

	m := rowToMemory(results[0])
	return &m, nil
}

func (ls *LanceStore) Update(id string, text string, meta map[string]any) error {
	ctx := context.Background()
	updates := map[string]interface{}{
		"updated_at": time.Now().Format(time.RFC3339),
	}
	if text != "" {
		updates["text"] = text
	}
	if v, ok := meta["category"]; ok {
		updates["category"] = fmt.Sprint(v)
	}
	if v, ok := meta["importance"]; ok {
		updates["importance"] = v
	}

	return ls.table.Update(ctx, fmt.Sprintf("id = '%s'", id), updates)
}

func (ls *LanceStore) Delete(id string) error {
	return ls.table.Delete(context.Background(), fmt.Sprintf("id = '%s'", id))
}

func (ls *LanceStore) DeleteByQuery(embedding []float32, threshold float32, scope string) (int, error) {
	results, err := ls.Search(embedding, 100, scope)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, r := range results {
		if r.Distance < threshold {
			if err := ls.Delete(r.ID); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

func (ls *LanceStore) RunDecay(cfg memory.DecayConfig) (int, error) {
	all, err := ls.All("")
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, m := range all {
		memory.ApplyDecay(&m, cfg)
		if m.Importance <= 0 {
			ls.Delete(m.ID)
			deleted++
		} else {
			ls.Update(m.ID, "", map[string]any{"importance": m.Importance})
		}
	}
	return deleted, nil
}

func (ls *LanceStore) All(scope string) ([]memory.Memory, error) {
	ctx := context.Background()
	where := ""
	if scope != "" {
		where = fmt.Sprintf("scope = '%s'", scope)
	}

	limit := 10000
	config := contracts.QueryConfig{
		Columns: []string{"id", "text", "category", "content_type", "scope", "importance", "access_count", "created_at", "updated_at", "last_access"},
		Limit:   &limit,
	}
	if where != "" {
		config.Where = where
	}

	results, err := ls.table.Select(ctx, config)
	if err != nil {
		return nil, err
	}

	out := make([]memory.Memory, 0, len(results))
	for _, row := range results {
		out = append(out, rowToMemory(row))
	}
	return out, nil
}

func (ls *LanceStore) Close() error {
	if ls.conn != nil {
		return ls.conn.Close()
	}
	return nil
}

// buildRecord создаёт Arrow Record из слайса Memory.
func buildRecord(mems []memory.Memory) arrow.Record {
	alloc := arrowmemory.NewGoAllocator()

	idBuilder := array.NewStringBuilder(alloc)
	textBuilder := array.NewStringBuilder(alloc)
	catBuilder := array.NewStringBuilder(alloc)
	ctBuilder := array.NewStringBuilder(alloc)
	scopeBuilder := array.NewStringBuilder(alloc)
	impBuilder := array.NewFloat64Builder(alloc)
	acBuilder := array.NewInt32Builder(alloc)
	createdBuilder := array.NewStringBuilder(alloc)
	updatedBuilder := array.NewStringBuilder(alloc)
	accessBuilder := array.NewStringBuilder(alloc)
	vecBuilder := array.NewFloat32Builder(alloc)

	for _, m := range mems {
		idBuilder.Append(m.ID)
		textBuilder.Append(m.Text)
		catBuilder.Append(string(m.Category))
		ctBuilder.Append(string(m.ContentType))
		scopeBuilder.Append(m.Scope)
		impBuilder.Append(m.Importance)
		acBuilder.Append(int32(m.AccessCount))
		createdBuilder.Append(m.CreatedAt.Format(time.RFC3339))
		updatedBuilder.Append(m.UpdatedAt.Format(time.RFC3339))
		accessBuilder.Append(m.LastAccess.Format(time.RFC3339))

		// Vector: flatten all floats
		for _, v := range m.Embedding {
			vecBuilder.Append(v)
		}
	}

	idArr := idBuilder.NewArray()
	textArr := textBuilder.NewArray()
	catArr := catBuilder.NewArray()
	ctArr := ctBuilder.NewArray()
	scopeArr := scopeBuilder.NewArray()
	impArr := impBuilder.NewArray()
	acArr := acBuilder.NewArray()
	createdArr := createdBuilder.NewArray()
	updatedArr := updatedBuilder.NewArray()
	accessArr := accessBuilder.NewArray()
	vecFlat := vecBuilder.NewArray()

	// Wrap flat float array into FixedSizeList
	vecListType := arrow.FixedSizeListOf(embeddingDims, arrow.PrimitiveTypes.Float32)
	vecArr := array.NewFixedSizeListData(
		array.NewData(vecListType, len(mems), []*arrowmemory.Buffer{nil}, []arrow.ArrayData{vecFlat.Data()}, 0, 0),
	)

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String},
		{Name: "text", Type: arrow.BinaryTypes.String},
		{Name: "category", Type: arrow.BinaryTypes.String},
		{Name: "content_type", Type: arrow.BinaryTypes.String},
		{Name: "scope", Type: arrow.BinaryTypes.String},
		{Name: "importance", Type: arrow.PrimitiveTypes.Float64},
		{Name: "access_count", Type: arrow.PrimitiveTypes.Int32},
		{Name: "created_at", Type: arrow.BinaryTypes.String},
		{Name: "updated_at", Type: arrow.BinaryTypes.String},
		{Name: "last_access", Type: arrow.BinaryTypes.String},
		{Name: "vector", Type: arrow.FixedSizeListOf(embeddingDims, arrow.PrimitiveTypes.Float32)},
	}, nil)

	return array.NewRecord(schema, []arrow.Array{
		idArr, textArr, catArr, ctArr, scopeArr, impArr, acArr,
		createdArr, updatedArr, accessArr, vecArr,
	}, int64(len(mems)))
}

func rowToMemory(row map[string]interface{}) memory.Memory {
	m := memory.Memory{}

	if v, ok := row["id"].(string); ok {
		m.ID = v
	}
	if v, ok := row["text"].(string); ok {
		m.Text = v
	}
	if v, ok := row["category"].(string); ok {
		m.Category = memory.Category(v)
	}
	if v, ok := row["content_type"].(string); ok {
		m.ContentType = memory.ContentType(v)
	}
	if v, ok := row["scope"].(string); ok {
		m.Scope = v
	}
	if v, ok := row["importance"]; ok {
		switch f := v.(type) {
		case float64:
			m.Importance = f
		case json.Number:
			m.Importance, _ = f.Float64()
		}
	}
	if v, ok := row["access_count"]; ok {
		switch n := v.(type) {
		case int32:
			m.AccessCount = int(n)
		case float64:
			m.AccessCount = int(n)
		}
	}
	if v, ok := row["created_at"].(string); ok {
		m.CreatedAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := row["updated_at"].(string); ok {
		m.UpdatedAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := row["last_access"].(string); ok {
		m.LastAccess, _ = time.Parse(time.RFC3339, v)
	}

	return m
}

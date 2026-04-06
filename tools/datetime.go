package tools

import (
	"encoding/json"
	"time"
)

type DateTime struct{}

func (d DateTime) Name() string        { return "get_datetime" }
func (d DateTime) Description() string { return "Returns current date and time" }

func (d DateTime) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (d DateTime) Execute(_ json.RawMessage) (string, error) {
	return time.Now().Format("2006-01-02 15:04:05 MST"), nil
}

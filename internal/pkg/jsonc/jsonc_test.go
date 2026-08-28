package jsonc

import (
	"encoding/json"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	input := []byte(`{
		// 这是一个单行注释
		"api": {
			"tag": "api", /* 多行注释 */
			"listen": "127.0.0.1:8080"
		},
		"url": "https://example.com/test//not_a_comment"
	}`)

	cleaned := StripJSONC(input)
	var js map[string]interface{}
	if err := json.Unmarshal(cleaned, &js); err != nil {
		t.Fatalf("Unmarshal failed: %v\nCleaned output:\n%s", err, string(cleaned))
	}

	if js["url"] != "https://example.com/test//not_a_comment" {
		t.Errorf("URL in string got corrupted: %v", js["url"])
	}
}

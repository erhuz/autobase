package redact

import (
	"encoding/json"
	"regexp"
	"strings"
)

const Mask = "[REDACTED]"

var (
	authorizationPattern = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer|basic)?\s*[^\s,;]+`)
	secretPattern        = regexp.MustCompile(`(?i)\b(password|passwd|passphrase|secret|token|credential|private[_-]?key|access[_-]?key|client[_-]?(?:ip|addr)|raw[_-]?sql|query[_-]?text|error[_-]?text)\b(\s*[:=]\s*)(?:"(?:\\.|[^"])*"|'[^']*'|[^\s,;]+)`)
	urlPasswordPattern   = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^:/@\s]+:)[^@\s]+@`)
)

// Value returns a JSON-compatible copy with sensitive fields and strings masked.
func Value(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			if sensitiveKey(key) {
				out[key] = Mask
			} else {
				out[key] = Value(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = Value(item)
		}
		return out
	case string:
		return Text(value)
	default:
		return value
	}
}

// JSON masks sensitive values in JSON. Invalid input becomes a masked string.
func JSON(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return []byte(`"` + Mask + `"`)
	}
	sanitized, err := json.Marshal(Value(value))
	if err != nil {
		return []byte(`"` + Mask + `"`)
	}
	return sanitized
}

// Text masks common structured secret forms in logs.
func Text(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var parsed any
		if json.Unmarshal([]byte(trimmed), &parsed) == nil {
			if sanitized, err := json.Marshal(Value(parsed)); err == nil {
				return string(sanitized)
			}
		}
	}
	value = authorizationPattern.ReplaceAllString(value, `${1}`+Mask)
	value = secretPattern.ReplaceAllString(value, `${1}${2}`+Mask)
	return urlPasswordPattern.ReplaceAllString(value, `${1}`+Mask+`@`)
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, suffix := range []string{"_id", "_name", "_ref", "_reference"} {
		if strings.HasSuffix(key, suffix) {
			return false
		}
	}
	for _, marker := range []string{
		"password", "passwd", "passphrase", "secret", "token", "authorization",
		"credential", "private_key", "access_key", "ssh_key",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	switch key {
	case "query", "sql", "statement", "raw_sql", "query_text", "client_ip", "client_addr",
		"remote_addr", "query_plan", "explain_plan", "comment", "comments", "error_text", "stderr":
		return true
	default:
		return false
	}
}

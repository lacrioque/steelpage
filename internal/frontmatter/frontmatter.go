package frontmatter

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	delim = []byte("---")
	bom   = []byte{0xEF, 0xBB, 0xBF}
)

func Split(raw []byte) (header, body []byte, hasHeader bool) {
	trimmed := bytes.TrimPrefix(raw, bom)
	if !bytes.HasPrefix(trimmed, append(delim, '\n')) && !bytes.HasPrefix(trimmed, append(delim, '\r', '\n')) {
		return nil, raw, false
	}

	after := trimmed[len(delim):]
	if len(after) > 0 && after[0] == '\r' {
		after = after[1:]
	}
	if len(after) > 0 && after[0] == '\n' {
		after = after[1:]
	}

	closeIdx := findClose(after)
	if closeIdx < 0 {
		return nil, raw, false
	}

	header = after[:closeIdx]

	bodyStart := closeIdx + len(delim)
	if bodyStart < len(after) && after[bodyStart] == '\r' {
		bodyStart++
	}
	if bodyStart < len(after) && after[bodyStart] == '\n' {
		bodyStart++
	}

	body = after[bodyStart:]
	return header, body, true
}

func findClose(after []byte) int {
	cursor := 0
	for cursor < len(after) {
		nl := bytes.IndexByte(after[cursor:], '\n')
		var line []byte
		var next int
		if nl < 0 {
			line = after[cursor:]
			next = len(after)
		} else {
			line = after[cursor : cursor+nl]
			next = cursor + nl + 1
		}
		trimmed := bytes.TrimRight(line, "\r")
		if bytes.Equal(trimmed, delim) {
			return cursor
		}
		cursor = next
	}
	return -1
}

func Parse(header []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(header)) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := yaml.Unmarshal(header, &out); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func Serialize(fm map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(fm); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ApplyUpdate(fm map[string]any, username string, now time.Time) {
	ts := now.Format(time.RFC3339)
	if v, ok := fm["created"]; !ok || v == nil || v == "" {
		fm["created"] = ts
	}
	fm["updated"] = ts
	fm["updated_by"] = username

	switch v := fm["version"].(type) {
	case int:
		fm["version"] = v + 1
	case int64:
		fm["version"] = int(v) + 1
	case float64:
		fm["version"] = int(v) + 1
	default:
		fm["version"] = 1
	}
}

func Recombine(fm map[string]any, body []byte) ([]byte, error) {
	headerYAML, err := Serialize(fm)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(headerYAML)
	if !bytes.HasSuffix(headerYAML, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString("---\n")
	buf.Write(body)
	return buf.Bytes(), nil
}

func Title(fm map[string]any, fallback string) string {
	if v, ok := fm["title"].(string); ok && v != "" {
		return v
	}
	return fallback
}

package command

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, stringValue(item))
		}
		return strings.Join(values, ", ")
	default:
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(value)
	}
}

func objectValue(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func arrayValue(value any) []any {
	items, _ := value.([]any)
	return items
}

func cloneObject(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func required(value string, flag string) (string, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		return value, nil
	}
	return "", newCommandError(
		"MISSING_REQUIRED_OPTION",
		"Missing required option: --"+flag+".",
		nil,
	)
}

func positiveInteger(value string, flag string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return "", newCommandError(
			"INVALID_OPTION",
			"--"+flag+" must be a positive integer.",
			nil,
		)
	}
	return strconv.Itoa(parsed), nil
}

// Package deps: 内部 JSON 工具.
package deps

import "encoding/json"

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
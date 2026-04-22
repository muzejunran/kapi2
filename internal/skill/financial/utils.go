package financial

import "fmt"

// getFloatFromMap 从 map 中获取 float64 值
func getFloatFromMap(params map[string]interface{}, key string) float64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case string:
			var f float64
			fmt.Sscanf(v, "%f", &f)
			return f
		}
	}
	return 0
}

// getStringFromMap 从 map 中获取 string 值
func getStringFromMap(params map[string]interface{}, key string) string {
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

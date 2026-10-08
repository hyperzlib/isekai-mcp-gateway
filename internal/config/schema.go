package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// GenerateSchemas derives the static/dynamic file schemas from the same types
// used by the strict decoder. Cross-field and environment checks remain in Validate.
func GenerateSchemas(dir string) error {
	for name, t := range map[string]reflect.Type{"server": reflect.TypeFor[ServerFile](), "app": reflect.TypeFor[AppFile]()} {
		s := schema(t)
		s["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		s["title"], s["x-reload"] = "Gateway "+name+" configuration", name == "app"
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, "gateway-config."+name+".schema.json"), append(b, '\n'), 0644); err != nil {
			return err
		}
	}
	return nil
}

func schema(t reflect.Type) map[string]any {
	s := map[string]any{}
	// RawMessage（如 JWKS 内部结构）不展开属性，保持任意对象。
	if t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{"type": "object", "additionalProperties": true}
	}
	// 指针类型在 JSON Schema 中同时接受基础类型与 null。
	if t.Kind() == reflect.Pointer {
		s = schema(t.Elem())
		s["type"] = []string{s["type"].(string), "null"}
		return s
	}
	switch t.Kind() {
	case reflect.Struct:
		p := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				property := schema(f.Type)
				// 标记 secret 的字段只允许写入，禁止从配置回读。
				if f.Tag.Get("secret") == "true" {
					property["writeOnly"] = true
				}
				p[name] = property
			}
		}
		s["type"], s["properties"], s["additionalProperties"] = "object", p, false
	case reflect.Slice:
		s["type"], s["items"] = "array", schema(t.Elem())
	case reflect.Map:
		s["type"], s["additionalProperties"] = "object", schema(t.Elem())
	case reflect.Bool:
		s["type"] = "boolean"
	case reflect.Int, reflect.Int64:
		s["type"] = "integer"
	default:
		s["type"] = "string"
	}
	if t == reflect.TypeFor[Site]() {
		p := s["properties"].(map[string]any)
		p["title"].(map[string]any)["maxLength"] = 256
		p["title"].(map[string]any)["default"] = DefaultSiteTitle
		p["iconUrl"].(map[string]any)["default"] = ""
	}
	return s
}

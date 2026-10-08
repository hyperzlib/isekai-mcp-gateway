package wiki

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

func resourceSegment(uri, template, marker string) (string, error) {
	u, err := url.Parse(uri)
	parts := strings.Split(template, marker)
	if err != nil || len(parts) != 2 || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(uri, "#") ||
		!strings.HasPrefix(uri, parts[0]) || !strings.HasSuffix(uri, parts[1]) {
		return "", errCode("NOT_FOUND", "Resource was not found.")
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(uri, parts[0]), parts[1])
	if raw == "" || strings.ContainsAny(raw, "/?#") {
		return "", errCode("NOT_FOUND", "Resource was not found.")
	}
	return raw, nil
}

func resourceID(raw string) (int64, error) {
	if strings.Trim(raw, "0123456789") != "" {
		return 0, errCode("NOT_FOUND", "Resource was not found.")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errCode("NOT_FOUND", "Resource was not found.")
	}
	return id, nil
}

// ReadResourceInput 只接收已注册模板的选择器，筛选和续页由对应 Tool 处理。
func ReadResourceInput(uri, template, operation string) (Input, error) {
	if IsPageRead(operation) {
		return PageResourceInput(uri, template)
	}
	if operation == "get_my_contributions" || operation == "get_recent_changes" {
		if uri != template {
			return Input{}, errCode("NOT_FOUND", "Resource was not found.")
		}
		return Input{}, nil
	}
	if operation != "get_user_contributions" {
		return Input{}, errCode("NOT_FOUND", "Resource was not found.")
	}
	if strings.Contains(template, "{user_id}") {
		raw, err := resourceSegment(uri, template, "{user_id}")
		if err != nil {
			return Input{}, err
		}
		id, err := resourceID(raw)
		return Input{UserID: id}, err
	}
	raw, err := resourceSegment(uri, template, "{encoded_username}")
	if err != nil {
		return Input{}, err
	}
	username, err := url.PathUnescape(raw)
	if err != nil || !utf8.ValidString(username) || !validUsername(username) {
		return Input{}, errCode("NOT_FOUND", "Resource was not found.")
	}
	return Input{Username: username}, nil
}

func encodedSegment(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

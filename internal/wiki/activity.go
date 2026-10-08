package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func IsActivityRead(name string) bool {
	return name == "get_user_contributions" || name == "get_my_contributions" || name == "get_recent_changes"
}

func validUsername(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 255 && !strings.ContainsAny(value, "|#\x00\r\n")
}

func userSelector(i Input, required bool) error {
	if i.UserID < 0 || i.UserID > 0 && i.Username != "" || required && i.UserID == 0 && i.Username == "" {
		return errCode("INVALID_REQUEST", "Supply exactly one positive user_id or username.")
	}
	if i.Username != "" && !validUsername(i.Username) {
		return errCode("INVALID_REQUEST", "Invalid username.")
	}
	return nil
}

func (s *Service) readActivity(ctx context.Context, c *Call, name string, i Input) (Object, error) {
	limit, err := positive(i.Limit, 20, 100)
	if err != nil {
		return nil, err
	}
	direction, err := activityRange(i)
	if err != nil {
		return nil, err
	}
	namespaces, err := namespaceValues(i.Namespaces)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(i.Tag, "|\x00\r\n") || len(i.Tag) > 255 {
		return nil, errCode("INVALID_REQUEST", "Invalid tag.")
	}

	// 本人贡献始终使用认证上下文中的 MediaWiki 本地身份，不接受调用方覆盖。
	if name == "get_my_contributions" {
		if i.UserID != 0 || i.Username != "" {
			return nil, errCode("INVALID_REQUEST", "Current-user contributions do not accept a user selector.")
		}
		if c.Credentials.UserID <= 0 {
			return nil, errCode("INVALID_TOKEN", "Current Wiki user is unavailable.")
		}
		i.UserID = c.Credentials.UserID
	}
	if err = userSelector(i, name != "get_recent_changes"); err != nil {
		return nil, err
	}

	q := url.Values{"action": {"query"}}
	prefix, list, continuation := "uc", "usercontribs", "uccontinue"
	showValues := "minor !minor new !new top !top patrolled !patrolled autopatrolled !autopatrolled"
	if name == "get_recent_changes" {
		prefix, list, continuation = "rc", "recentchanges", "rccontinue"
		showValues = "minor !minor bot !bot anon !anon redirect !redirect patrolled !patrolled autopatrolled !autopatrolled"
		types := i.Types
		if len(types) == 0 {
			types = []string{"edit", "new"}
		}
		value, err := enumValues(types, "edit new log categorize external")
		if err != nil {
			return nil, err
		}
		q.Set("rctype", value)
		q.Set("rcprop", "title|ids|timestamp|user|userid|comment|sizes|flags|tags|loginfo")
		if i.UserID > 0 {
			// MediaWiki 1.43 的 rcuser 不接受 #ID，先用核心 users API 解析真实用户名。
			username, err := s.activityUsername(ctx, c, i.UserID)
			if err != nil {
				return nil, err
			}
			q.Set("rcuser", username)
		} else if i.Username != "" {
			q.Set("rcuser", i.Username)
		}
	} else {
		q.Set("ucprop", "ids|title|timestamp|comment|size|sizediff|flags|tags")
		if i.UserID > 0 {
			q.Set("ucuserids", fmt.Sprint(i.UserID))
		} else {
			q.Set("ucuser", i.Username)
		}
	}
	show, err := enumValues(i.Show, showValues)
	if err != nil {
		return nil, err
	}
	q.Set("list", list)
	q.Set(prefix+"limit", strconv.Itoa(limit))
	q.Set(prefix+"dir", direction)
	for key, value := range map[string]string{"start": i.Start, "end": i.End, "namespace": namespaces, "show": show, "tag": i.Tag} {
		if value != "" {
			q.Set(prefix+key, value)
		}
	}

	// 指纹由实际发往上游的参数生成，默认值与显式默认值具有相同语义。
	out, err := s.queryList(ctx, c, name, q, q, i, continuation)
	if err != nil {
		return nil, err
	}
	addPageLinks(out["results"].([]any))
	if i.UserID > 0 {
		out["user_id"] = i.UserID
	}
	if i.Username != "" {
		out["username"] = i.Username
	}
	resourceURI := "wiki://recent-changes"
	if name == "get_my_contributions" {
		resourceURI = "wiki://me/contributions"
	} else if name == "get_user_contributions" {
		if i.UserID > 0 {
			resourceURI = fmt.Sprintf("wiki://users/by-id/%d/contributions", i.UserID)
		} else {
			resourceURI = "wiki://users/by-name/" + encodedSegment(i.Username) + "/contributions"
		}
	}
	out["resources"] = Object{"default": resourceURI}
	return out, nil
}

func (s *Service) activityUsername(ctx context.Context, c *Call, id int64) (string, error) {
	v, err := c.Client.Query(ctx, c.Credentials, url.Values{
		"action": {"query"}, "list": {"users"}, "ususerids": {fmt.Sprint(id)},
	})
	if err != nil {
		return "", err
	}
	items, err := listResult(v, "users")
	if err != nil {
		return "", err
	}
	if len(items) != 1 {
		return "", errCode("UPSTREAM_RESPONSE_INVALID", "Wiki did not return exactly one user.")
	}
	user := object(items[0])
	if _, missing := user["missing"]; missing {
		return "", errCode("NOT_FOUND", "Wiki user was not found.")
	}
	if number(user["userid"]) != id || !validUsername(str(user["name"])) {
		return "", errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned invalid user metadata.")
	}
	return str(user["name"]), nil
}

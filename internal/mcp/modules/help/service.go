package help

import (
	"bytes"
	"isekai-mcp-gateway/internal/failure"
	"slices"
	"strings"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/mcp/modules/help/domain"
	"isekai-mcp-gateway/internal/templates"
)

type Service struct{ templates templates.TextRenderer }

func New(renderer templates.TextRenderer) *Service { return &Service{templates: renderer} }

func (s *Service) Read(topic string) (domain.Result, error) {
	// 未指定时默认返回索引页。
	if topic == "" {
		topic = "index"
	}

	// 校验清单恒含扩展专属主题，帮助正文模板内部按开关分支渲染；
	// 展示清单仅列出已启用的扩展，保持关闭时无专属残留。
	cfg := config.GetConfig()
	topics := []string{
		"index", "connection", "authorization", "reading",
		"activity", "editing", "wikitext", "page-creation", "patch", "recovery",
		"confirmation", "uploads", "moderation", "default-writing-style",
	}
	visible := topics
	if cfg == nil || !cfg.Static.Extensions.Moderation.Enabled {
		visible = slices.DeleteFunc(slices.Clone(topics), func(t string) bool { return t == "moderation" })
	}
	// 白名单校验可拦截 "../index" 一类的路径穿越输入。
	if !slices.Contains(topics, topic) {
		return domain.Result{}, failure.New("UNKNOWN_HELP_TOPIC", "use wiki.help with topic=index", "help.read", nil)
	}

	var body bytes.Buffer
	features := []string{
		"oauth", "help", "logout", "me", "wikitext", "apply-patch",
		"wiki-read", "wiki-activity", "wiki-edit", "wiki-mutation", "uploads",
	}
	if cfg != nil && cfg.Static.Extensions.Moderation.Enabled {
		features = append(features, "moderation")
	}
	if cfg != nil && cfg.Static.Extensions.ACL.Enabled {
		features = append(features, "page-acl")
	}
	operations := strings.Fields("wiki.help wiki.logout wiki.me wiki.get_latest_page wiki.get_baseline_page wiki.search wiki.get_history wiki.get_revision wiki.compare_revisions wiki.get_backlinks wiki.list_category wiki.get_user wiki.begin_edit wiki.begin_create wiki.list_edits wiki.get_edit_status wiki.get_edit_content wiki.search_edit_content wiki.update_edit wiki.preview_edit wiki.get_edit_diff wiki.get_edit_conflict wiki.commit_edit wiki.pull_edit wiki.close_edit wiki.move_page wiki.delete_page wiki.prepare_upload wiki.commit_upload")
	operations = append(operations,
		"wiki.get_file_urls",
		"wiki.get_user_contributions", "wiki.get_my_contributions", "wiki.get_recent_changes",
		"wiki.get_page_structure", "wiki.get_template_data", "wiki.get_page_dependencies", "wiki.get_transclusions")

	// 按主题渲染模板，并把展示主题、特性、操作与扩展开关注入上下文。
	err := s.templates.RenderText(&body, "help/"+topic+".md", map[string]any{"Topics": visible, "Features": features, "Operations": operations, "ModerationEnabled": cfg != nil && cfg.Static.Extensions.Moderation.Enabled})
	if err != nil {
		return domain.Result{}, failure.Wrap("help.render", err)
	}

	// 帮助内容同样受文本大小上限约束。
	if cfg != nil && int64(body.Len()) > cfg.App.Results.TextMax {
		return domain.Result{}, failure.New("RESOURCE_TOO_LARGE", "Help content exceeds the configured text limit.", "wiki", nil)
	}
	return domain.Result{Topic: topic, URI: "wiki://help/" + topic, Content: body.String(), AvailableOperations: operations, EnabledFeatures: features}, nil
}

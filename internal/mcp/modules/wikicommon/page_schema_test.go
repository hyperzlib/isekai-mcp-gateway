package wikicommon

import "testing"

func TestResolvePageURLSchema(t *testing.T) {
	s := resolveSchema(Schema("resolve_page_url"))
	for _, input := range []map[string]any{{"url": "/首页"}, {"url": "https://wiki.test/index.php?title=首页"}} {
		if err := s.Validate(input); err != nil {
			t.Fatal(err)
		}
	}
	o := resolveSchema(OutputSchema("resolve_page_url"))
	if err := o.Validate(map[string]any{"title": "首页", "page_id": float64(1), "exists": true}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []map[string]any{{"title": "首页", "page_id": float64(1)}, {"title": "首页", "page_id": float64(1), "exists": nil}, {"title": "首页", "page_id": float64(1), "exists": true, "extra": 1}} {
		if o.Validate(value) == nil {
			t.Fatalf("invalid output accepted: %#v", value)
		}
	}
}

func TestPageReadSelectors(t *testing.T) {
	for _, name := range []string{"get_latest_page", "get_baseline_page", "get_history", "get_backlinks", "list_subpages"} {
		s := resolveSchema(Schema(name))
		for _, input := range []map[string]any{{"title": "帮助:编辑/章节"}, {"page_id": float64(123)}} {
			if err := s.Validate(input); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		for _, input := range []map[string]any{{}, {"title": ""}, {"page_id": float64(0)}, {"title": "A", "page_id": float64(123)}, {"page": map[string]any{"title": "A"}}} {
			if err := s.Validate(input); err == nil {
				t.Fatalf("%s accepted %#v", name, input)
			}
		}
	}
	if err := resolveSchema(Schema("begin_edit")).Validate(map[string]any{"page": map[string]any{"page_id": float64(123)}}); err != nil {
		t.Fatal(err)
	}
}

func TestAdditionalReadSchemas(t *testing.T) {
	for _, name := range []string{"get_page_structure", "get_page_dependencies"} {
		s := resolveSchema(Schema(name))
		for _, input := range []map[string]any{
			{"title": "帮助:编辑/章节"}, {"page_id": float64(3)}, {"revision_id": float64(10)},
		} {
			if err := s.Validate(input); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		for _, input := range []map[string]any{
			{}, {"title": "A", "revision_id": float64(10)}, {"page_id": float64(3), "revision_id": float64(10)},
			{"revision_id": float64(0)}, {"title": "A", "limit": float64(101)},
		} {
			if err := s.Validate(input); err == nil {
				t.Fatalf("%s accepted %#v", name, input)
			}
		}
	}
	for _, name := range []string{"get_template_data", "get_transclusions", "list_subpages"} {
		s := resolveSchema(Schema(name))
		if err := s.Validate(map[string]any{"title": "Template:人物"}); err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(map[string]any{"revision_id": float64(10)}); err == nil {
			t.Fatalf("%s accepted unsupported revision", name)
		}
	}
	for _, name := range []string{"get_my_contributions", "get_recent_changes"} {
		if err := resolveSchema(Schema(name)).Validate(map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := resolveSchema(Schema("get_my_contributions")).Validate(map[string]any{"user_id": float64(17)}); err == nil {
		t.Fatal("current user could be overridden")
	}
	if err := resolveSchema(Schema("get_user_contributions")).Validate(map[string]any{}); err == nil {
		t.Fatal("contributions accepted no user")
	}
	if err := resolveSchema(Schema("get_user_contributions")).Validate(map[string]any{"username": "A", "user_id": float64(17)}); err == nil {
		t.Fatal("contributions accepted both user selectors")
	}
	if err := resolveSchema(Schema("get_recent_changes")).Validate(map[string]any{"types": []any{"invalid"}}); err == nil {
		t.Fatal("invalid change type accepted")
	}
	if err := resolveSchema(Schema("get_page_dependencies")).Validate(map[string]any{"title": "A", "dependency_types": []any{"links", "links"}}); err == nil {
		t.Fatal("duplicate dependency types accepted")
	}
}

func TestFileURLSchemas(t *testing.T) {
	input := resolveSchema(Schema("get_file_urls"))
	if err := input.Validate(map[string]any{"titles": []any{"文件:示例.png", "File:Example.pdf"}}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []map[string]any{{}, {"titles": []any{}}, {"titles": "文件:示例.png"}, {"titles": []any{1}}} {
		if input.Validate(v) == nil {
			t.Fatalf("accepted %#v", v)
		}
	}
	for _, name := range []string{"get_file_urls", "get_latest_page", "get_baseline_page"} {
		output := resolveSchema(OutputSchema(name))
		if err := output.Validate(map[string]any{"file_urls": map[string]any{"文件:示例.png": "https://files.test/image.png"}}); err != nil {
			t.Fatal(err)
		}
		if output.Validate(map[string]any{"file_urls": map[string]any{"文件:示例.png": 42}}) == nil {
			t.Fatal("invalid URL type accepted")
		}
	}
}

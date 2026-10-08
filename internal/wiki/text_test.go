package wiki

import (
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"strings"
	"testing"
)

func TestOrderedAtomicOperations(t *testing.T) {
	tests := []struct {
		name, input string
		ops         []Operation
		want, code  string
	}{
		{"replace", "a\nb", []Operation{{Type: "replace_content", Content: "x\ny"}}, "x\ny", ""},
		{"text exact", "aba", []Operation{{Type: "replace_text", OldText: "b", NewText: "xyz"}}, "axyza", ""},
		{"ambiguous", "a a", []Operation{{Type: "replace_text", OldText: "a", NewText: "b"}}, "", "MATCH_AMBIGUOUS"},
		{"lines", "a\nb\nc", []Operation{{Type: "replace_lines", StartLine: 2, EndLine: 2, Content: "x\ny\n"}, {Type: "delete_lines", StartLine: 4, EndLine: 4}}, "a\nx\ny", ""},
		{"empty insertion", "", []Operation{{Type: "insert_lines", Line: 1, Position: "before", Content: "hello\n"}}, "hello", ""},
		{"append", "a", []Operation{{Type: "insert_lines", Line: 1, Position: "after", Content: "b\nc"}}, "a\nb\nc", ""},
		{"range", "a", []Operation{{Type: "delete_lines", StartLine: 0, EndLine: 1}}, "", "LINE_RANGE_INVALID"},
		{"patch", "a\nb\nc", []Operation{{Type: "apply_patch", Patch: "*** Begin Patch\n@@\n a\n-b\n+x\n c\n*** End Patch"}}, "a\nx\nc", ""},
		{"patch no file headers", "a", []Operation{{Type: "apply_patch", Patch: "*** Begin Patch\n*** Update File: page\n@@\n-a\n+b\n*** End Patch"}}, "", "PATCH_INVALID"},
		{"patch ambiguous", "a\na", []Operation{{Type: "apply_patch", Patch: "*** Begin Patch\n@@\n-a\n+b\n*** End Patch"}}, "", "PATCH_MATCH_FAILED"},
		{"batch failure", "a", []Operation{{Type: "replace_content", Content: "b"}, {Type: "replace_text", OldText: "missing", NewText: "x"}}, "", "MATCH_NOT_FOUND"},
		{"normalize after batch", "a\r\nb\r\n", []Operation{{Type: "replace_lines", StartLine: 2, EndLine: 2, Content: "c\r\n"}}, "a\nc", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, e := Apply(tt.input, tt.ops, 1<<20)
			if tt.code != "" {
				if e == nil || failure.Describe(e).Code != tt.code {
					t.Fatalf("expected %s, got %v", tt.code, e)
				}
				return
			}
			if e != nil || got != tt.want {
				t.Fatalf("got %q %v want %q", got, e, tt.want)
			}
		})
	}
}
func TestPullRepresentsEveryDifference(t *testing.T) {
	for _, v := range [][2]string{{"", "new"}, {"old", ""}, {"a\nb\nc\nd", "a\nx\nc\ny"}, {"same", "same"}} {
		got := pullConflict(v[0], v[1], "token")
		ranges, valid := conflicts(got, "token")
		if !valid {
			t.Fatalf("invalid markers: %s", got)
		}
		if v[0] == v[1] && len(ranges) != 0 {
			t.Fatal("unchanged text has conflicts")
		}
		if v[0] != v[1] && len(ranges) == 0 {
			t.Fatal("difference omitted")
		}
		// 按冲突标记重建 current/edited 两侧文本，验证差异未丢失。
		current, edited := []string{}, []string{}
		state := "outside"
		for _, line := range lines(got) {
			switch line {
			case "<<<<<<< current:token":
				state = "current"
			case "=======":
				state = "edited"
			case ">>>>>>> edited:token":
				state = "outside"
			default:
				if state != "edited" {
					current = append(current, line)
				}
				if state != "current" {
					edited = append(edited, line)
				}
			}
		}
		if strings.Join(current, "\n") != v[0] || strings.Join(edited, "\n") != v[1] {
			t.Fatalf("lost text: %s", got)
		}
	}
}
func TestDamagedMarkersBlockCommit(t *testing.T) {
	// 残缺、错序或 token 不匹配的标记都必须被判为无效。
	for _, v := range []string{"<<<<<<< current:t\na", "=======", "a\n>>>>>>> edited:t", "<<<<<<< current:x\na\n=======\nb\n>>>>>>> edited:t"} {
		_, valid := conflicts(v, "t")
		if valid {
			t.Fatalf("damaged markers accepted: %s", v)
		}
	}
}
func TestUTF8FragmentsAndVersionCursor(t *testing.T) {
	if _, e := fragment("一二三", 1, 3, 100); e == nil {
		t.Fatal("invalid UTF8 offset accepted")
	}
	v, e := fragment("一二三", 0, 4, 100)
	if e != nil || v["content"] != "一" || v["next_offset"] != 3 {
		t.Fatalf("bad fragment %+v %v", v, e)
	}
	// 不同 owner 读取 cursor 必须被拒绝。
	raw := cursor(cursorData{Tool: "x", Owner: "a", Filter: "f"})
	if _, e = readCursor(raw, "x", "b", "f"); e == nil {
		t.Fatal("cross owner cursor accepted")
	}
	c := &Call{Config: config.Defaults()}
	e1 := Edit{ID: "a", Owner: "owner", Version: 3, Content: "a\nb"}
	v, e = search(e1, c, Input{Pattern: "a"})
	if e != nil || len(v["search_results"].([]Object)) != 1 {
		t.Fatal(v, e)
	}
	if _, e = search(e1, c, Input{Pattern: "(?<=a)b", Regex: true}); e == nil {
		t.Fatal("unsupported regex accepted")
	}
}

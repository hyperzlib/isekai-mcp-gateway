package wiki

import (
	"fmt"
	"isekai-mcp-gateway/internal/failure"
	"strings"
	"unicode/utf8"
)

func lines(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, "\n")
}
func operationLines(v string) []string { return lines(strings.TrimSuffix(v, "\n")) }
func Apply(text string, ops []Operation, maxBytes int64) (string, error) {
	text = normalizeLines(text)
	// 所有操作统一按换行归一化后顺序执行，任一失败即整体放弃（原子语义）。
	for index, op := range ops {
		op.Content = normalizeLines(op.Content)
		op.OldText = normalizeLines(op.OldText)
		op.NewText = normalizeLines(op.NewText)
		op.Patch = normalizeLines(op.Patch)
		next, e := applyOne(text, op)
		if e != nil {
			f := failure.Describe(e)
			d := Object{"operation_index": index}
			for k, v := range f.Details {
				d[k] = v
			}
			return "", detail(f.Code, "An edit operation failed; no changes were saved. Read the affected lines and retry.", d)
		}
		// 每次操作后校验 UTF-8 与大小上限，超限立即中断。
		if !utf8.ValidString(next) || int64(len(next)) > maxBytes {
			return "", errCode("CONTENT_TOO_LARGE", "Edited content exceeds the limit or contains invalid UTF-8.")
		}
		text = next
	}
	// 去除结尾的空白与空字符等控制字符后返回。
	return strings.TrimRight(text, " \t\n\r\x00\x0b"), nil
}
func normalizeLines(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, "\r\n", "\n"), "\r", "\n")
}
func applyOne(text string, op Operation) (string, error) {
	switch op.Type {
	case "replace_content":
		return op.Content, nil
	case "replace_text":
		// old_text 必须唯一匹配，0 次报未找到，多次报歧义。
		if op.OldText == "" {
			return "", errCode("INVALID_REQUEST", "old_text must be nonempty.")
		}
		n := strings.Count(text, op.OldText)
		if n == 0 {
			return "", errCode("MATCH_NOT_FOUND", "MATCH_NOT_FOUND")
		}
		if n != 1 {
			return "", errCode("MATCH_AMBIGUOUS", "MATCH_AMBIGUOUS")
		}
		return strings.Replace(text, op.OldText, op.NewText, 1), nil
	case "replace_lines", "delete_lines", "insert_lines":
		a := lines(text)
		// 行号对外为 1-based，这里转成切片下标 [start,end)。
		start, end := op.StartLine-1, op.EndLine
		if op.Type == "insert_lines" {
			if op.Content == "" || op.Position != "before" && op.Position != "after" {
				return "", errCode("INVALID_REQUEST", "Insertion requires content and before/after.")
			}
			// 空文档允许在首行前插入作为建页特例。
			if len(a) == 0 && op.Line == 1 && op.Position == "before" {
				return strings.Join(operationLines(op.Content), "\n"), nil
			}
			if op.Line < 1 || op.Line > len(a) {
				return "", errCode("LINE_RANGE_INVALID", "LINE_RANGE_INVALID")
			}
			start = op.Line - 1
			if op.Position == "after" {
				start++
			}
			end = start
		} else if start < 0 || end <= start || end > len(a) {
			return "", errCode("LINE_RANGE_INVALID", "LINE_RANGE_INVALID")
		}
		// delete 用空中间块，其余用内容替换 [start,end) 区间。
		var middle []string
		if op.Type != "delete_lines" {
			middle = operationLines(op.Content)
		}
		out := append([]string{}, a[:start]...)
		out = append(out, middle...)
		out = append(out, a[end:]...)
		return strings.Join(out, "\n"), nil
	case "apply_patch":
		return applyPatch(text, op.Patch)
	}
	return "", errCode("INVALID_REQUEST", "Unknown operation type.")
}
func applyPatch(text, patch string) (string, error) {
	// patch 须以 Begin/End Patch 包裹，不接受文件头，行数不足以判定时直接拒绝。
	a := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	if len(a) < 5 || a[0] != "*** Begin Patch" || a[len(a)-1] != "*** End Patch" {
		return "", errCode("PATCH_INVALID", "Patch requires Begin/End Patch markers and no file headers.")
	}
	var old, new []string
	hunk := -1
	changed := false
	// flush 结算当前 hunk：旧块需非空且确有增删，并保证在原文中唯一匹配。
	flush := func() error {
		if hunk < 0 {
			return nil
		}
		if len(old) == 0 || !changed {
			return errCode("PATCH_INVALID", "Patch hunk requires a nonempty old block and a change.")
		}
		needle := strings.Join(old, "\n")
		n := strings.Count(text, needle)
		if n != 1 {
			return detail("PATCH_MATCH_FAILED", "Patch hunk must match exactly once.", Object{"hunk_index": hunk, "matches": n})
		}
		text = strings.Replace(text, needle, strings.Join(new, "\n"), 1)
		return nil
	}
	// 逐行解析：@@ 开启新 hunk，前缀 ' ' 保留、'-' 删除、'+' 新增。
	for _, line := range a[1 : len(a)-1] {
		if line == "@@" {
			if e := flush(); e != nil {
				return "", e
			}
			hunk++
			old = nil
			new = nil
			changed = false
			continue
		}
		if hunk < 0 || len(line) == 0 {
			return "", errCode("PATCH_INVALID", "Each hunk starts with @@ and each line requires a prefix.")
		}
		switch line[0] {
		case ' ':
			old = append(old, line[1:])
			new = append(new, line[1:])
		case '-':
			old = append(old, line[1:])
			changed = true
		case '+':
			new = append(new, line[1:])
			changed = true
		default:
			return "", errCode("PATCH_INVALID", "Unsupported patch directive.")
		}
	}
	if hunk < 0 {
		return "", errCode("PATCH_INVALID", "Patch contains no hunks.")
	}
	if e := flush(); e != nil {
		return "", e
	}
	return text, nil
}
func conflicts(text, token string) ([]LineRange, bool) {
	a := lines(text)
	var ranges []LineRange
	start, separator := 0, false
	valid := true
	// 状态机扫描冲突标记：<<<<<<< 起始、======= 分隔、>>>>>>> 结束。
	for i, line := range a {
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			if token == "" || line != "<<<<<<< current:"+token || start != 0 {
				valid = false
			}
			start = i + 1
			separator = false
		case strings.HasPrefix(line, "======="):
			if start == 0 || separator || line != "=======" {
				valid = false
			}
			separator = true
		case strings.HasPrefix(line, ">>>>>>>"):
			if start == 0 || !separator || token == "" || line != ">>>>>>> edited:"+token {
				valid = false
			}
			ranges = append(ranges, LineRange{start, i + 1})
			start = 0
			separator = false
		}
	}
	// 标记未闭合视为无效。
	if start != 0 {
		valid = false
	}
	return ranges, valid
}

// A bounded LCS finds all changed blocks; very large comparisons use a single
// block so memory is bounded while every difference remains represented.
type change struct{ AStart, AEnd, BStart, BEnd int }

func changes(a, b []string) []change {
	// 先剥离公共前后缀，缩小后续 LCS 的规模。
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	ae, be := len(a), len(b)
	for ae > prefix && be > prefix && a[ae-1] == b[be-1] {
		ae--
		be--
	}
	if ae == prefix && be == prefix {
		return nil
	}
	n, m := ae-prefix, be-prefix
	// 超过阈值不再分配 DP 表，退化成单个整块差异以限制内存。
	if int64(n+1)*int64(m+1) > 2000000 {
		return []change{{prefix, ae, prefix, be}}
	}
	// 反向 DP 求 LCS 长度表，用于回溯对齐。
	dp := make([]int32, (n+1)*(m+1))
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			k := i*(m+1) + j
			if a[prefix+i] == b[prefix+j] {
				dp[k] = dp[(i+1)*(m+1)+j+1] + 1
			} else {
				dp[k] = max(dp[(i+1)*(m+1)+j], dp[i*(m+1)+j+1])
			}
		}
	}
	// 依据 DP 回溯，跳过相等行并合并连续差异为变更块。
	var out []change
	i, j := 0, 0
	var cur *change
	flush := func() {
		if cur != nil {
			cur.AEnd = prefix + i
			cur.BEnd = prefix + j
			out = append(out, *cur)
			cur = nil
		}
	}
	for i < n || j < m {
		if i < n && j < m && a[prefix+i] == b[prefix+j] {
			flush()
			i++
			j++
			continue
		}
		if cur == nil {
			cur = &change{AStart: prefix + i, BStart: prefix + j}
		}
		if j < m && (i == n || dp[i*(m+1)+j+1] >= dp[(i+1)*(m+1)+j]) {
			j++
		} else {
			i++
		}
	}
	flush()
	return out
}
func pullConflict(current, edited, token string) string {
	a, b := lines(current), lines(edited)
	var out []string
	pos := 0
	// 把每个变更块包成 current/edited 冲突标记，未变的行原样保留。
	for _, h := range changes(a, b) {
		out = append(out, a[pos:h.AStart]...)
		out = append(out, "<<<<<<< current:"+token)
		out = append(out, a[h.AStart:h.AEnd]...)
		out = append(out, "=======")
		out = append(out, b[h.BStart:h.BEnd]...)
		out = append(out, ">>>>>>> edited:"+token)
		pos = h.AEnd
	}
	out = append(out, a[pos:]...)
	return strings.Join(out, "\n")
}
func unified(origin, edited string, context int) string {
	a, b := lines(origin), lines(edited)
	hs := changes(a, b)
	if len(hs) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("--- origin\n+++ edited\n")
	// 把相隔不超过 2*context 的变更块合并进同一个 hunk。
	for first := 0; first < len(hs); {
		last := first
		for last+1 < len(hs) && hs[last+1].AStart <= hs[last].AEnd+2*context {
			last++
		}
		// 计算含上下文的 hunk 起止范围；空范围时起始行回退一位以免越界。
		as, bs := max(0, hs[first].AStart-context), max(0, hs[first].BStart-context)
		ae, be := min(len(a), hs[last].AEnd+context), min(len(b), hs[last].BEnd+context)
		oldStart, newStart := as+1, bs+1
		if ae == as {
			oldStart = as
		}
		if be == bs {
			newStart = bs
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", oldStart, ae-as, newStart, be-bs)
		pos := as
		for k := first; k <= last; k++ {
			h := hs[k]
			for _, v := range a[pos:h.AStart] {
				out.WriteString(" " + v + "\n")
			}
			for _, v := range a[h.AStart:h.AEnd] {
				out.WriteString("-" + v + "\n")
			}
			for _, v := range b[h.BStart:h.BEnd] {
				out.WriteString("+" + v + "\n")
			}
			pos = h.AEnd
		}
		for _, v := range a[pos:ae] {
			out.WriteString(" " + v + "\n")
		}
		first = last + 1
	}
	return out.String()
}

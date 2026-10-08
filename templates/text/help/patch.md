# Single-page patches (apply_patch)

To make localized changes to a draft, call `wiki.update_edit` with an entry of the form `{"type":"apply_patch","patch":"patch text"}` in `operations`. Supply the target session's `edit_id` and the latest `base_version`. Displaying a patch in a response does not modify the draft.

This tool uses a single-page variant of the GPT/Codex apply_patch format. Follow the syntax below exactly when generating a patch.

## Format rules

1. The first line must be `*** Begin Patch` and the last line must be `*** End Patch`, with no leading spaces.
2. Between these markers, include one or more hunks. Start each hunk with `@@` on a line by itself. Do not append a heading, anchor text, or line numbers to `@@`.
3. The first character of every line inside a hunk is an operation prefix; the remaining characters are page content:
   - One ASCII space: preserve the original line as context.
   - `-`: delete this original line.
   - `+`: add this line.
4. Every hunk must contain at least one original line (prefixed with a space or `-`) and at least one addition or deletion (prefixed with `+` or `-`). A hunk containing only `+` lines is invalid. Include adjacent original lines as context when inserting text. Use `replace_content` for an empty draft.
5. Blank content lines also require a prefix: use a single space to preserve a blank line, `-` to delete one, or `+` to add one. Start the next hunk directly with `@@`; do not insert unprefixed blank lines between hunks.
6. Do not include file headers or other diff directives, including `*** Update File:`, `*** Add File:`, `*** Delete File:`, `*** Move to:`, `*** End of File`, `diff --git`, `---` / `+++` file headers, `@@ -1,3 +1,3 @@`, or `\ No newline at end of file`.
7. The `patch` field must contain only the patch text, without Markdown fences, explanatory prose, shell commands, or heredoc wrappers.

The operation prefix is exactly one character. Preserve the original content's indentation, spaces, tabs, punctuation, and letter case. For example, delete the original line `* 项目` with `-* 项目`; preserve the original line `+1` with ` +1`. Exclude display-only line numbers from the patch.

## Constructing hunks that match

Read the target content from the current draft with `get_edit_content`. If needed, locate it with `search_edit_content`, then read the complete relevant lines. Use the content returned by these tools rather than reconstructing it from memory.

For each hunk, remove the operation prefixes and join all space-prefixed and `-` lines in order with newlines to form the old block. Join all space-prefixed and `+` lines in the same way to form the new block. The tool replaces the uniquely matching old block with the new block.

The old block must occur exactly once in the current content. Matching is exact: the tool does not perform fuzzy matching or ignore whitespace differences. Usually include 2–3 context lines before and after the change. If the block is still ambiguous, extend the context to a heading or paragraph that identifies it uniquely. Do not replace omitted content with ellipses or invent context lines. Avoid using only blank lines or short common fragments as anchors.

Hunks execute in the order written. Each subsequent hunk matches the content produced by earlier hunks. Entries in the same `operations` batch also execute in order. Combine adjacent or overlapping changes into one hunk. Use separate `@@` hunks for distant changes, ensuring that each old block matches uniquely when its hunk executes.

## Examples

The code fences below are for illustration only. Exclude them from the actual `patch` field.

Replace one line, preserving the heading and following line as context:

```text
*** Begin Patch
@@
 == 城市简介 ==
-这座城市建于第三纪元。
+这座城市建于第二纪元。
 城市位于河流北岸。
*** End Patch
```

Insert one content line and one blank line, using existing content as context:

```text
*** Begin Patch
@@
 == 交通 ==
 城内以步行为主。
+港口提供定期渡船。
+
 == 历史 ==
*** End Patch
```

Delete one line and modify another location using two separate hunks:

```text
*** Begin Patch
@@
 == 城市简介 ==
-待补充的临时说明。
 城市位于河流北岸。
@@
 == 人口 ==
-常住人口约一万人。
+常住人口约两万人。
 统计不包含旅客。
*** End Patch
```

Example JSON arguments for `wiki.update_edit` (replace `edit_id` and `base_version` with the current session's actual values):

```json
{
  "edit_id": "当前会话的 edit_id",
  "base_version": 3,
  "operations": [
    {
      "type": "apply_patch",
      "patch": "*** Begin Patch\n@@\n == 城市简介 ==\n-这座城市建于第三纪元。\n+这座城市建于第二纪元。\n 城市位于河流北岸。\n*** End Patch"
    }
  ]
}
```

In JSON, encode newlines as `\n` and escape double quotes and backslashes according to JSON rules. After decoding, `patch` must contain actual newline characters. Do not double-escape newlines as literal `\\n` sequences.

## Validation and error recovery

Before sending, check every hunk: it contains original content and at least one addition or deletion; every content line has a prefix; the old block is complete and unique; context comes from the current version; both patch markers are present; and there are no file headers or line numbers. If there is no actual change, do not send an empty patch.

For `PATCH_INVALID`, correct the format using the rules above. For `PATCH_MATCH_FAILED`, the old block did not match uniquely: if `matches = 0`, reread the target content and check the version and whitespace; if `matches > 1`, add context. When present in error details, `operation_index` and `hunk_index` are zero-based indexes identifying the failed operation and hunk respectively.

If any step fails, the entire `operations` batch is discarded; earlier operations in that batch have not been saved. Reread the status and affected content, then construct a patch using the latest `base_version`. If unique matching is not possible, read the exact line range and use `replace_lines`. After success, use the returned new version for subsequent edits and inspect the result with `get_edit_diff` and `preview_edit`.

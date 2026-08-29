package jsonc

import "bytes"

// StripJSONC 过滤 JSON 文本中的 // 单行注释与 /* */ 多行注释，同时保护字符串内部的引号与斜杠
func StripJSONC(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))

	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false

	n := len(data)
	for i := 0; i < n; i++ {
		ch := data[i]

		// 处理单行注释状态
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				out.WriteByte(ch)
			}
			continue
		}

		// 处理多行块注释状态
		if inBlockComment {
			if ch == '*' && i+1 < n && data[i+1] == '/' {
				inBlockComment = false
				i++ // 跳过 '/'
			}
			continue
		}

		// 处理字符串字面量内部状态
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}

		// 检查进入字符串
		if ch == '"' {
			inString = true
			escaped = false
			out.WriteByte(ch)
			continue
		}

		// 检查单行注释 //
		if ch == '/' && i+1 < n && data[i+1] == '/' {
			inLineComment = true
			i++ // 跳过第二个 '/'
			continue
		}

		// 检查多行注释 /*
		if ch == '/' && i+1 < n && data[i+1] == '*' {
			inBlockComment = true
			i++ // 跳过 '*'
			continue
		}

		out.WriteByte(ch)
	}

	return out.Bytes()
}

package messageprofile

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const listShapeMarker = "\ue000"

func Clean(text string, botHandle string) string {
	text = stripTransport(text)
	text = stripHandle(text, strings.TrimSpace(botHandle))
	text = preserveListShape(text)
	text = stripNumericIdentifiers(text)
	return strings.Join(strings.Fields(text), " ")
}

func stripTransport(text string) string {
	var cleaned strings.Builder
	cleaned.Grow(len(text))
	for index := 0; index < len(text); {
		switch {
		case strings.HasPrefix(text[index:], "```"):
			index = skipDelimitedText(text, index, "```")
			cleaned.WriteByte(' ')
		case text[index] == '`':
			index = skipDelimitedText(text, index, "`")
			cleaned.WriteByte(' ')
		case text[index] == '<' && structuredEnd(text, index) > index:
			index = structuredEnd(text, index)
			cleaned.WriteByte(' ')
		case text[index] == ':' && customSymbolEnd(text, index) > index:
			index = customSymbolEnd(text, index)
			cleaned.WriteByte(' ')
		case isURLStart(text, index):
			index = skipURL(text, index)
			cleaned.WriteByte(' ')
		default:
			r, size := utf8.DecodeRuneInString(text[index:])
			cleaned.WriteRune(r)
			index += size
		}
	}
	return cleaned.String()
}

func skipDelimitedText(text string, start int, marker string) int {
	restStart := start + len(marker)
	end := strings.Index(text[restStart:], marker)
	if end < 0 {
		return len(text)
	}
	return restStart + end + len(marker)
}

func structuredEnd(text string, start int) int {
	relativeEnd := strings.IndexByte(text[start+1:], '>')
	if relativeEnd < 0 {
		return -1
	}
	end := start + relativeEnd + 1
	if !isStructuredContent(text[start+1 : end]) {
		return -1
	}
	return end + 1
}

func isStructuredContent(content string) bool {
	for _, prefix := range []string{"@!", "@&", "@", "#"} {
		if strings.HasPrefix(content, prefix) && allDigits(content[len(prefix):]) {
			return true
		}
	}
	if strings.HasPrefix(content, "t:") {
		return isTimestampTag(content[2:])
	}
	separator := strings.LastIndexByte(content, ':')
	if separator < 1 || !allDigits(content[separator+1:]) {
		return false
	}
	prefix := content[:separator]
	return strings.HasPrefix(prefix, ":") || strings.HasPrefix(prefix, "a:") ||
		strings.HasPrefix(prefix, "/")
}

func isTimestampTag(content string) bool {
	parts := strings.Split(content, ":")
	if len(parts) == 1 {
		return allDigits(parts[0])
	}
	if len(parts) != 2 || !allDigits(parts[0]) {
		return false
	}
	switch parts[1] {
	case "t", "T", "d", "D", "f", "F", "R":
		return true
	default:
		return false
	}
}

func customSymbolEnd(text string, start int) int {
	relativeEnd := strings.IndexByte(text[start+1:], ':')
	if relativeEnd < 1 {
		return -1
	}
	end := start + relativeEnd + 1
	for _, value := range text[start+1 : end] {
		if unicode.IsSpace(value) {
			return -1
		}
	}
	return end + 1
}

func isURLStart(text string, index int) bool {
	if index > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:index])
		if isWordRune(previous) {
			return false
		}
	}
	remaining := text[index:]
	return hasFoldPrefix(remaining, "https://") || hasFoldPrefix(remaining, "http://") ||
		hasFoldPrefix(remaining, "www.") || hasFoldPrefix(remaining, "discord.gg/") ||
		hasFoldPrefix(remaining, "discord.com/invite/")
}

func skipURL(text string, start int) int {
	for start < len(text) {
		value, size := utf8.DecodeRuneInString(text[start:])
		if unicode.IsSpace(value) {
			break
		}
		start += size
	}
	return start
}

func preserveListShape(text string) string {
	lines := strings.Split(text, "\n")
	markers := 0
	for index, line := range lines {
		if content, ok := stripListMarker(line); ok {
			lines[index] = content
			markers++
		}
	}
	if markers < 2 {
		return text
	}
	return listShapeMarker + " " + strings.Join(lines, "\n")
}

func stripListMarker(line string) (string, bool) {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	if len(trimmed) >= 2 && (trimmed[0] == '-' || trimmed[0] == '*') {
		if space, _ := utf8.DecodeRuneInString(trimmed[1:]); unicode.IsSpace(space) {
			return strings.TrimLeftFunc(trimmed[1:], unicode.IsSpace), true
		}
	}
	end := 0
	for end < len(trimmed) && trimmed[end] >= '0' && trimmed[end] <= '9' {
		end++
	}
	if end == 0 || end+1 >= len(trimmed) || (trimmed[end] != '.' && trimmed[end] != ')') {
		return line, false
	}
	remainder := trimmed[end+1:]
	space, _ := utf8.DecodeRuneInString(remainder)
	if !unicode.IsSpace(space) {
		return line, false
	}
	return strings.TrimLeftFunc(remainder, unicode.IsSpace), true
}

func stripHandle(text string, handle string) string {
	if handle == "" {
		return text
	}
	var cleaned strings.Builder
	cleaned.Grow(len(text))
	for index := 0; index < len(text); {
		end := index + len(handle)
		if end <= len(text) && strings.EqualFold(text[index:end], handle) && wordBoundaryBefore(text, index) && wordBoundaryAfter(text, end) {
			cleaned.WriteByte(' ')
			index = end
			continue
		}
		value, size := utf8.DecodeRuneInString(text[index:])
		cleaned.WriteRune(value)
		index += size
	}
	return cleaned.String()
}

func stripNumericIdentifiers(text string) string {
	var cleaned strings.Builder
	cleaned.Grow(len(text))
	for index := 0; index < len(text); {
		value, size := utf8.DecodeRuneInString(text[index:])
		if !unicode.IsDigit(value) {
			cleaned.WriteRune(value)
			index += size
			continue
		}
		end := index + size
		for end < len(text) {
			next, nextSize := utf8.DecodeRuneInString(text[end:])
			if !unicode.IsDigit(next) {
				break
			}
			end += nextSize
		}
		if wordBoundaryBefore(text, index) && wordBoundaryAfter(text, end) {
			cleaned.WriteByte(' ')
		} else {
			cleaned.WriteString(text[index:end])
		}
		index = end
	}
	return cleaned.String()
}

func wordBoundaryBefore(text string, index int) bool {
	if index == 0 {
		return true
	}
	value, _ := utf8.DecodeLastRuneInString(text[:index])
	return !isWordRune(value)
}

func wordBoundaryAfter(text string, index int) bool {
	if index == len(text) {
		return true
	}
	value, _ := utf8.DecodeRuneInString(text[index:])
	return !isWordRune(value)
}

func isWordRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_'
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, item := range value {
		if !unicode.IsDigit(item) {
			return false
		}
	}
	return true
}

func hasFoldPrefix(text string, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}

package main

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/longbridgeapp/opencc"
)

// The patterns this file uses are compiled once. A chapter page runs
// stripTags and decodeEntities over thousands of fragments, and compiling a
// regular expression per call was the hot path of the whole parse.
var (
	reTags        = regexp.MustCompile(`(?s)<[^>]+>`)
	reNumericRef  = regexp.MustCompile(`&#(x?[0-9A-Fa-f]+);`)
	reWhitespace  = regexp.MustCompile(`\s+`)
	reIdentityBad = regexp.MustCompile(`[　\s\-—_:：,，。！？；、"'“”‘’（）()\[\]【】《》<>·]+`)
)

// firstMatch returns the first capture group of the pattern in s.
func firstMatch(pattern, s string) (string, bool) {
	re, err := regexp.Compile("(?is)" + pattern)
	if err != nil {
		return "", false
	}
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

// allMatches returns the capture groups of every match of the pattern.
func allMatches(pattern, s string) [][]string {
	re, err := regexp.Compile("(?is)" + pattern)
	if err != nil {
		return nil
	}
	matches := re.FindAllStringSubmatch(s, -1)
	out := make([][]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out = append(out, m[1:])
		}
	}
	return out
}

// removeMatches removes what the pattern matches.
func removeMatches(pattern, s string) string {
	re, err := regexp.Compile("(?is)" + pattern)
	if err != nil {
		return s
	}
	return re.ReplaceAllString(s, "")
}

// replaceMatches replaces what the pattern matches.
func replaceMatches(pattern, s, replacement string) string {
	re, err := regexp.Compile("(?is)" + pattern)
	if err != nil {
		return s
	}
	return re.ReplaceAllString(s, replacement)
}

// stripTags removes HTML tags.
func stripTags(s string) string {
	return reTags.ReplaceAllString(s, "")
}

// entityReplacer is the named entities the sites use, resolved in one pass.
var entityReplacer = strings.NewReplacer(
	"&nbsp;", " ",
	"&ensp;", " ",
	"&emsp;", " ",
	"&quot;", `"`,
	"&apos;", "'",
	"&lt;", "<",
	"&gt;", ">",
	"&amp;", "&",
	"&#39;", "'",
)

// decodeEntities turns the HTML entities of s into their characters.
func decodeEntities(s string) string {
	s = entityReplacer.Replace(s)
	return reNumericRef.ReplaceAllStringFunc(s, func(m string) string {
		token := m[2 : len(m)-1]
		base := 10
		if strings.HasPrefix(strings.ToLower(token), "x") {
			base = 16
			token = token[1:]
		}
		value, err := strconv.ParseInt(token, base, 32)
		if err != nil || value < 0 || value > 0x10FFFF {
			return m
		}
		return string(rune(value))
	})
}

// collapseNewlines normalizes line breaks and keeps at most one blank line.
func collapseNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// trimParagraph trims whitespace, including the full-width space.
func trimParagraph(s string) string {
	return strings.Trim(s, " \t\n\r\u3000")
}

var (
	openccOnce sync.Once
	openccConv *opencc.OpenCC
)

// toSimplified converts Traditional Chinese text to Simplified Chinese.
func toSimplified(s string) string {
	openccOnce.Do(func() {
		if c, err := opencc.New("t2s"); err == nil {
			openccConv = c
		}
	})
	if openccConv == nil || s == "" {
		return s
	}
	out, err := openccConv.Convert(s)
	if err != nil {
		return s
	}
	return out
}

// xmlEscaped escapes text for an XML or XHTML document.
func xmlEscaped(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// safeFileName replaces the characters a file name cannot hold.
func safeFileName(s, fallback string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\?%*|"<>:`, r) {
			return '-'
		}
		return r
	}, s)
	cleaned = trimParagraph(cleaned)
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

// lines splits text into its non-empty trimmed lines.
func lines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = trimParagraph(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// paragraphsFrom turns an HTML fragment into paragraphs of plain text.
func paragraphsFrom(htmlFragment string) []string {
	withBreaks := removeMatches(`<!--.*?-->`, htmlFragment)
	withBreaks = removeMatches(`<script.*?</script>`, withBreaks)
	withBreaks = removeMatches(`<style.*?</style>`, withBreaks)
	withBreaks = removeMatches(`<noscript.*?</noscript>`, withBreaks)
	withBreaks = replaceMatches(`<br\s*/?>`, withBreaks, "\n")
	withBreaks = replaceMatches(`</(p|div|section|article|li)>`, withBreaks, "\n")

	plain := decodeEntities(stripTags(withBreaks))
	plain = collapseNewlines(plain)

	var out []string
	for _, line := range strings.Split(plain, "\n") {
		if line = trimParagraph(line); line != "" && !isNoiseParagraph(line) {
			out = append(out, line)
		}
	}
	return out
}

// isNoiseParagraph reports whether a line is site chrome rather than prose.
func isNoiseParagraph(line string) bool {
	trimmed := trimParagraph(line)
	if trimmed == "" || trimmed == "()" || trimmed == "（）" || trimmed == ">" {
		return true
	}
	for _, token := range noiseTokens {
		if strings.Contains(trimmed, token) {
			return true
		}
	}
	return false
}

// noiseTokens are the site chrome strings the three sites wrap around a
// chapter. They are checked on every line of every page.
var noiseTokens = []string{
	"上一章", "下一章", "返回目录", "返回书页", "我的书架", "将本书加入书架",
	"章节错误", "点此举报", "返回顶部", "fontbig", "PT文学", "书籍介绍",
	"收藏本书", "加入书签", "69书吧", "最新网址", "请收藏", "手机用户请浏览",
	"笔趣阁", "纯文字在线阅读", "获取最新章节更新", "还有更新耶", "sto9.com",
	"𝘴𝘵𝘰9.𝘤𝘰𝘮", "看最新章节", "无弹窗", "首页 >", "還有更新耶", "獲取最新章節更新", "繁体最新章节",
}

// removeLeadingTitles removes paragraphs that repeat the chapter title.
func removeLeadingTitles(paragraphs []string, title string, identity func(string) string) []string {
	normalized := identity(title)
	if normalized == "" {
		return paragraphs
	}
	out := paragraphs
	for i := 0; i < 2 && len(out) > 0; i++ {
		if identity(out[0]) != normalized {
			break
		}
		out = out[1:]
	}
	return out
}

// identityTitle normalizes a title for comparison.
func identityTitle(s string) string {
	s = identityHeading(s)
	return strings.ToLower(reIdentityBad.ReplaceAllString(s, ""))
}

// identityHeading normalizes a heading for comparison.
func identityHeading(s string) string {
	return reWhitespace.ReplaceAllString(s, "")
}

// chapterHTML builds the XHTML body of a chapter from its paragraphs.
func chapterHTML(ch Chapter) string {
	paragraphs := strings.Split(ch.PlainText, "\n\n")
	var b strings.Builder
	for _, p := range paragraphs {
		if strings.TrimSpace(p) == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(xmlEscaped(p))
		b.WriteString("</p>\n")
	}
	return b.String()
}

// chapterFrom builds a chapter from its title and paragraphs.
func chapterFrom(title string, paragraphs []string) Chapter {
	if title == "" {
		title = "未命名章节"
	}
	return Chapter{Title: title, PlainText: strings.Join(paragraphs, "\n\n")}
}

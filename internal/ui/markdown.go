package ui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	reBold   = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	reCode   = regexp.MustCompile("`([^`\n]+)`")
	reItalic = regexp.MustCompile(`(^|[\s(])_([^_\n]+)_($|[\s).,;:])`)
	reHead   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reBullet = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	reNum    = regexp.MustCompile(`^(\s*)(\d+)[.)]\s+(.*)$`)
)

// inline styles **bold**, `code`, _italic_ on one line.
func inline(s string) string {
	s = reCode.ReplaceAllStringFunc(s, func(m string) string {
		return fg(colCode).Render(m[1 : len(m)-1])
	})
	s = reBold.ReplaceAllStringFunc(s, func(m string) string {
		return lipgloss.NewStyle().Bold(true).Render(m[2 : len(m)-2])
	})
	s = reItalic.ReplaceAllString(s, "$1"+lipgloss.NewStyle().Italic(true).Render("$2")+"$3")
	return s
}

// renderMarkdown is a small, dependency-free renderer: headings, lists, fenced code, inline styles.
func renderMarkdown(src string, width int) string {
	var out []string
	inFence := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		switch {
		case inFence:
			out = append(out, fg(colCode).Render("  "+line))
		case reHead.MatchString(line):
			m := reHead.FindStringSubmatch(line)
			out = append(out, lipgloss.NewStyle().Bold(true).Underline(len(m[1]) == 1).Render(inline(m[2])))
		case reBullet.MatchString(line):
			m := reBullet.FindStringSubmatch(line)
			out = append(out, wrapHanging(m[1]+fg(colDim).Render("• "), inline(m[2]), width))
		case reNum.MatchString(line):
			m := reNum.FindStringSubmatch(line)
			out = append(out, wrapHanging(m[1]+fg(colDim).Render(m[2]+". "), inline(m[3]), width))
		default:
			out = append(out, wrap(inline(line), width))
		}
	}
	return strings.Join(out, "\n")
}

// wrapHanging wraps text after a list marker, indenting continuation lines under the text.
func wrapHanging(marker, text string, width int) string {
	w := lipgloss.Width(marker)
	lines := strings.Split(wrap(text, width-w), "\n")
	for i, l := range lines {
		if i == 0 {
			lines[i] = marker + l
		} else {
			lines[i] = strings.Repeat(" ", w) + l
		}
	}
	return strings.Join(lines, "\n")
}

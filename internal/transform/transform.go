package transform

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"regexp"
	"strings"
)

var (
	keepComment  = regexp.MustCompile(`^//(go:|line\b|export\b|sys\b|sysnb\b|extern\b|nolint|cgo)|^/[*] *#|^// \+build`)
	multiNewline = regexp.MustCompile(`(?:\r?\n){3,}`)
)

type commentRange struct {
	start int
	end   int
}

func deletable(src []byte) []commentRange {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var sc scanner.Scanner
	sc.Init(file, src, nil, scanner.ScanComments)
	var out []commentRange
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			return out
		}
		if tok != token.COMMENT || strings.HasPrefix(lit, "#!") || keepComment.MatchString(lit) {
			continue
		}
		start := file.Offset(pos)
		out = append(out, commentRange{start: start, end: start + len(lit)})
	}
}

func StripComments(src []byte) []byte {
	if strings.Contains(string(src), "\"C\"") {
		return src
	}
	comments := deletable(src)
	if len(comments) == 0 {
		return src
	}
	out := make([]byte, 0, len(src))
	cursor := 0
	for _, comment := range comments {
		before := src[cursor:comment.start]
		lineStart := bytes.LastIndexByte(before, '\n') + 1
		afterEnd := comment.end
		if afterEnd < len(src) && src[afterEnd] == '\n' {
			afterEnd++
		}
		if strings.TrimSpace(string(before[lineStart:])) == "" {
			out = append(out, before[:lineStart]...)
			cursor = afterEnd
			continue
		}
		out = append(out, before...)
		cursor = comment.end
	}
	return append(out, src[cursor:]...)
}

func bodySpans(src []byte) []commentRange {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	var spans []commentRange
	ast.Inspect(parsed, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if ok && fn.Body != nil {
			spans = append(spans, commentRange{
				start: fset.Position(fn.Body.Lbrace).Offset,
				end:   fset.Position(fn.Body.Rbrace).Offset,
			})
		}
		return true
	})
	return spans
}

func inSpan(spans []commentRange, off int) bool {
	for _, span := range spans {
		if off > span.start && off < span.end {
			return true
		}
	}
	return false
}

func removeBodyBlanks(src []byte) []byte {
	spans := bodySpans(src)
	if len(spans) == 0 {
		return src
	}
	out := make([]byte, 0, len(src))
	lineStart := 0
	for pos := 0; pos <= len(src); pos++ {
		if pos != len(src) && src[pos] != '\n' {
			continue
		}
		line := src[lineStart:pos]
		drop := len(bytes.TrimSpace(line)) == 0 && inSpan(spans, lineStart)
		if !drop {
			out = append(out, line...)
			if pos < len(src) {
				out = append(out, '\n')
			}
		}
		lineStart = pos + 1
	}
	return out
}

func Compact(src []byte) []byte {
	return multiNewline.ReplaceAll(removeBodyBlanks(src), []byte("\n\n"))
}

package main

import (
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
)

var parseError = errors.New("parse error")

func parseHTML(bs []byte) (string, error) {
	var p parser
	if err := p.parse(string(bs)); err != nil {
		return "", err
	}
	chunks := make([]string, len(p.ts))
	for i, t := range p.ts {
		chunks[i] = t.toHTML()
	}
	return strings.Join(chunks, ""), nil
}

type text struct {
	text string
	bg   string
	fg   string
	bold bool
}

func (t *text) toHTML() string {
	escaped := html.EscapeString(t.text)
	var styles []string
	if len(t.bg) > 0 {
		styles = append(styles, "background-color: "+t.bg)
	}
	if len(t.fg) > 0 {
		styles = append(styles, "color: "+t.fg)
	}
	if t.bold {
		styles = append(styles, "font-weight: bold")
	}
	if len(styles) > 0 {
		style := strings.Join(styles, "; ")
		return fmt.Sprintf("<span style=\"%v\">%v</span>", style, escaped)
	} else {
		return escaped
	}
}

func parse(in string) ([]text, error) {
	if len(in) == 0 {
		return []text{}, nil
	}
	p := &parser{}
	err := p.parse(in)
	return p.ts, err
}

type parseState func(c rune) (parseState, error)

type parser struct {
	state parseState
	ts    []text
	b     strings.Builder
	c     strings.Builder

	bg   string
	fg   string
	bold bool
}

func (p *parser) append() {
	p.ts = append(p.ts, text{
		text: p.b.String(),
		bg:   p.bg,
		fg:   p.fg,
		bold: p.bold,
	})
	p.b.Reset()
}

func (p *parser) parse(in string) (err error) {
	p.ts = []text{}
	p.state = p.parsePlain
	for _, c := range in {
		p.state, err = p.state(c)
		if err != nil {
			return
		}
	}
	p.append()
	return
}

func (p *parser) parseCSI(c rune) (parseState, error) {
	if c == 'm' {
		strs := strings.Split(p.c.String(), ";")
		p.c.Reset()
		codes := make([]int, len(strs))
		for i, str := range strs {
			var err error
			codes[i], err = strconv.Atoi(str)
			if err != nil {
				return nil, err
			}
		}
		for i := 0; i < len(codes); i++ {
			switch c := codes[i]; {
			case 0 == c:
				p.bg = ""
				p.fg = ""
				p.bold = false
			case 1 == c:
				p.bold = true
			case 30 <= c && c <= 37:
				p.fg = LowColors[c-30]
			case 38 == c:
				if i < len(codes)-2 {
					i++
					if 5 == codes[i] {
						i++
						p.fg = color8bit(codes[i])
					}
				} else {
					return nil, parseError
				}
			case 40 <= c && c <= 47:
				p.bg = LowColors[c-40]
			case 48 == c:
				if i < len(codes)-2 {
					i++
					if 5 == codes[i] {
						i++
						p.bg = color8bit(codes[i])
					}
				} else {
					return nil, parseError
				}
			case 90 <= c && c <= 97:
				p.fg = HighColors[c-90]
			case 100 <= c && c <= 107:
				p.bg = HighColors[c-100]
			}
		}
		return p.parsePlain, nil
	} else {
		p.c.WriteRune(c)
		return p.parseCSI, nil
	}
}

func (p *parser) parseESC(c rune) (parseState, error) {
	if c == '[' {
		return p.parseCSI, nil
	}
	return nil, parseError
}

func (p *parser) parsePlain(c rune) (parseState, error) {
	if c == '\x1b' {
		if p.b.Len() > 0 {
			p.append()
		}
		return p.parseESC, nil
	}
	p.b.WriteRune(c)
	return p.parsePlain, nil
}

var LowColors = []string{
	"#303030", // black
	"#800000", // red
	"#008000", // green
	"#808000", // yellow
	"#000080", // blue
	"#800080", // magenta
	"#008080", // cyan
	"#c0c0c0", // white
}

var HighColors = []string{
	"#808080", // black
	"#ff0000", // red
	"#00ff00", // green
	"#ffff00", // yellow
	"#0000ff", // blue
	"#ff00ff", // magenta
	"#00ffff", // cyan
	"#ffffff", // white
}

var GrayscaleColors = []string{
	"#080808", "#121212", "#1c1c1c", "#262626", "#303030", "#3a3a3a",
	"#444444", "#4e4e4e", "#585858", "#626262", "#6c6c6c", "#767676",
	"#808080", "#8a8a8a", "#9e9e9e", "#9e9e9e", "#a8a8a8", "#b2b2b2",
	"#bcbcbc", "#c6c6c6", "#d0d0d0", "#dadada", "#e4e4e4", "#eeeeee",
}

var CubeColors = []string{"00", "5f", "87", "af", "d7", "ff"}

func color8bit(c int) string {
	switch {
	case 0 <= c && c <= 7:
		return LowColors[c]
	case 8 <= c && c <= 15:
		return HighColors[c-8]
	case 16 <= c && c <= 231:
		r := (c - 16) / 36
		g := (c - 16 - 36*r) / 6
		b := c - 16 - 36*r - 6*g
		return "#" + CubeColors[r] + CubeColors[g] + CubeColors[b]
	case 232 <= c && c <= 255:
		return GrayscaleColors[c-232]
	default:
		return ""
	}
}

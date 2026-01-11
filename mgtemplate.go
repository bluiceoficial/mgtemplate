// Copyright (C) 2026 Murilo Gomes Julio
// SPDX-License-Identifier: MIT
//
// Site: https://mugomes.github.io

package mgtemplate

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

type MGTemplate struct {
	source       string
	context      map[string]any
	sectionCalls map[string][]map[string]any
}

var cleanupRegexps = []*regexp.Regexp{
	regexp.MustCompile(`\[\[[\w _-]+\]\].*?\[\[\/[\w _-]+\]\]`),
	regexp.MustCompile(`\[\[[\w _-]+\]\]`),
	regexp.MustCompile(`\[\[\/[\w _-]+\]\]`),
	regexp.MustCompile(`\{\{__[^}]+__\}\}`),
	regexp.MustCompile(`\{\{[^}]+\}\}`),
}

func ReadFile(path string) (*MGTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &MGTemplate{
		source:       string(data),
		context:      map[string]any{},
		sectionCalls: map[string][]map[string]any{},
	}, nil
}

// Variáveis

func (t *MGTemplate) Var(name string, value any) {
	t.context[name] = value
}

func (t *MGTemplate) VarExists(name string) bool {
	return strings.Contains(t.source, "{{"+name+"}}")
}

// Include

func (t *MGTemplate) IncludeFile(varname, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	t.source = strings.ReplaceAll(
		t.source,
		"{{"+varname+"}}",
		string(data),
	)

	return nil
}

// Seções

func (t *MGTemplate) Section(name string) {
	ctx := map[string]any{}
	for k, v := range t.context {
		ctx[k] = v
	}
	t.sectionCalls[name] = append(t.sectionCalls[name], ctx)
}

// Render

func (t *MGTemplate) Render() string {
	out := t.source

	// 1️⃣ Renderiza apenas seções chamadas
	for name, calls := range t.sectionCalls {
		out = t.renderSection(out, name, calls)
	}

	// 2️⃣ REMOVE blocos que NÃO foram chamados (comportamento PHP)
	out = t.removeUnusedSections(out)

	// 3️⃣ Interpola variáveis
	out = t.interpolate(out)

	// 4️⃣ Limpeza final
	return t.cleanup(out)
}

func (t *MGTemplate) removeUnusedSections(html string) string {
	var out strings.Builder
	pos := 0
	for {
		relStart := strings.Index(html[pos:], "[[")
		if relStart < 0 {
			out.WriteString(html[pos:])
			break
		}
		start := pos + relStart

		endNameRel := strings.Index(html[start+2:], "]]")
		if endNameRel < 0 {
			// no closing brackets for opening tag -> copy rest and break
			out.WriteString(html[pos:])
			break
		}

		name := strings.TrimSpace(html[start+2 : start+2+endNameRel])
		openEnd := start + 2 + endNameRel + 2 // position after opening tag

		// If section was called, keep the opening tag as-is (copy up to opening end)
		if _, ok := t.sectionCalls[name]; ok {
			out.WriteString(html[pos:openEnd])
			pos = openEnd
			continue
		}

		// procurar fechamento correspondente a partir do ponto da abertura
		closeTag := "[[/" + name + "]]"
		closeRel := strings.Index(html[openEnd:], closeTag)
		if closeRel < 0 {
			// abertura órfã -> remove apenas a tag de abertura (não o conteúdo)
			out.WriteString(html[pos:start])
			pos = openEnd
			continue
		}

		// pular bloco inteiro (remove desde abertura até fechamento)
		pos = openEnd + closeRel + len(closeTag)
	}
	return out.String()
}

func (t *MGTemplate) renderSection(html, name string, calls []map[string]any) string {
	open := "[[" + name + "]]"
	close := "[[/" + name + "]]"

	for {
		startRel := strings.Index(html, open)
		if startRel < 0 {
			break
		}
		start := startRel

		// Busca fechamento a partir do fim da abertura
		searchFrom := start + len(open)
		endRel := strings.Index(html[searchFrom:], close)
		if endRel < 0 {
			break
		}
		end := searchFrom + endRel

		body := html[start+len(open) : end]
		result := strings.Builder{}

		for _, ctx := range calls {
			original := t.context
			t.context = ctx

			// renderiza sub-blocos antes
			inner := body
			for sub, subCalls := range t.sectionCalls {
				if sub != name {
					inner = t.renderSection(inner, sub, subCalls)
				}
			}

			result.WriteString(t.interpolate(inner))
			t.context = original
		}

		html = html[:start] + result.String() + html[end+len(close):]
	}

	return html
}

// Engine

func (t *MGTemplate) interpolate(input string) string {
	var out strings.Builder

	for {
		start := strings.Index(input, "{{")
		if start < 0 {
			out.WriteString(input)
			break
		}

		out.WriteString(input[:start])

		end := strings.Index(input[start:], "}}")
		if end < 0 {
			out.WriteString(input)
			break
		}

		expr := strings.TrimSpace(input[start+2 : start+end])
		out.WriteString(t.evaluate(expr))

		input = input[start+end+2:]
	}

	return out.String()
}

func (t *MGTemplate) evaluate(expr string) string {
	parts := strings.Split(expr, "|")
	value := t.resolve(strings.TrimSpace(parts[0]))

	for _, f := range parts[1:] {
		value = transform(value, strings.TrimSpace(f))
	}

	return value
}

// Cleanup

func (t *MGTemplate) cleanup(html string) string {
	for _, r := range cleanupRegexps {
		html = r.ReplaceAllString(html, "")
	}
	return html
}

// Resolve

func (t *MGTemplate) resolve(path string) string {
	segments := strings.Split(path, ".")
	current, ok := t.context[segments[0]]
	if !ok {
		return ""
	}

	for _, seg := range segments[1:] {

		// array / map
		if m, ok := current.(map[string]any); ok {
			current = m[seg]
			continue
		}

		// valor simples → NÃO quebra
		rv := reflect.ValueOf(current)
		if rv.Kind() != reflect.Struct && rv.Kind() != reflect.Pointer {
			return ""
		}

		if rv.Kind() == reflect.Pointer {
			rv = rv.Elem()
		}

		// propriedade pública
		if rv.Kind() == reflect.Struct {
			field := rv.FieldByNameFunc(func(name string) bool {
				return normalize(name) == normalize(seg)
			})

			if field.IsValid() {
				current = field.Interface()
				continue
			}

			// getter GetX
			for i := 0; i < rv.NumMethod(); i++ {
				m := rv.Type().Method(i)
				if strings.HasPrefix(m.Name, "Get") &&
					normalize(m.Name[3:]) == normalize(seg) {

					res := rv.Method(i).Call(nil)
					if len(res) > 0 {
						current = res[0].Interface()
						goto next
					}
				}
			}
		}

		return ""
	next:
	}

	if current == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(current))
}

// Filtros

func transform(value, op string) string {
	switch strings.ToLower(op) {
	case "upper":
		return strings.ToUpper(value)
	case "lower":
		return strings.ToLower(value)
	case "trim":
		return strings.TrimSpace(value)
	default:
		return value
	}
}

// Utils

func normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// Copyright (C) 2026 Murilo Gomes Julio
// SPDX-License-Identifier: MIT
//
// Site: https://mugomes.github.io

package mgtemplate

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"unicode"
)

type MGTemplate struct {
	source string

	// variável global do template
	context map[string]any

	// seção encontrados no template
	sections map[string]string

	// valores acumulados por seção (snapshot do contexto)
	sectionsAccum map[string][]map[string]any
}

// ReadFile carrega um arquivo de template
func ReadFile(path string) (*MGTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &MGTemplate{
		source:        string(data),
		context:       map[string]any{},
		sections:      map[string]string{},
		sectionsAccum: map[string][]map[string]any{},
	}, nil
}

// IncludeFile inclui o conteúdo de outro arquivo no template
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

// Var define uma variável no contexto global
func (t *MGTemplate) Var(name string, value any) {
	t.context[name] = value
}

// VarExists verifica se a variável aparece no template
func (t *MGTemplate) VarExists(name string) bool {
	return strings.Contains(t.source, "{{"+name+"}}")
}

// Section registra uma repetição de seção
func (t *MGTemplate) Section(name string) {
	// se o seção ainda não foi identificado, extrai do template
	corpo, existe := t.sections[name]
	if !existe {
		abertura := "[[" + name + "]]"
		fechamento := "[[/" + name + "]]"

		inicio := strings.Index(t.source, abertura)
		fim := strings.Index(t.source, fechamento)
		if inicio == -1 || fim == -1 || fim < inicio {
			return
		}

		corpo = t.source[inicio+len(abertura) : fim]
		t.sections[name] = corpo
	}

	// cria uma cópia do contexto atual (escopo isolado)
	snapshot := copiarContexto(t.context)
	t.sectionsAccum[name] = append(t.sectionsAccum[name], snapshot)
}

// Render gera o HTML final
func (t *MGTemplate) Render() string {
	saida := t.source

	// resolve seções (do interno para o externo)
	for {
		houveMudanca := false

		for nome, corpo := range t.sections {
			abertura := "[[" + nome + "]]"
			fechamento := "[[/" + nome + "]]"

			if strings.Contains(saida, abertura) {
				var resultado strings.Builder

				for _, ctx := range t.sectionsAccum[nome] {
					resultado.WriteString(
						substituirVariaveis(corpo, ctx),
					)
				}

				saida = substituirSecao(
					saida,
					abertura,
					fechamento,
					resultado.String(),
				)

				houveMudanca = true
			}
		}

		if !houveMudanca {
			break
		}
	}

	// resolve variáveis finais fora de seções
	return substituirVariaveis(saida, t.context)
}

// INTERNAL HELPERS

// cria uma cópia do contexto
func copiarContexto(orig map[string]any) map[string]any {
	copia := make(map[string]any, len(orig))
	for k, v := range orig {
		copia[k] = v
	}
	return copia
}

// substitui [[SECTION]] pelo conteúdo renderizado
func substituirSecao(texto, abertura, fechamento, valor string) string {
	for {
		a := strings.Index(texto, abertura)
		b := strings.Index(texto, fechamento)
		if a == -1 || b == -1 || b < a {
			break
		}
		texto = texto[:a] + valor + texto[b+len(fechamento):]
	}
	return texto
}

// substitui {{variavel}} usando o contexto informado
func substituirVariaveis(texto string, ctx map[string]any) string {
	var saida strings.Builder

	for {
		inicio := strings.Index(texto, "{{")
		if inicio == -1 {
			saida.WriteString(texto)
			break
		}

		saida.WriteString(texto[:inicio])

		fim := strings.Index(texto[inicio:], "}}")
		if fim == -1 {
			saida.WriteString(texto)
			break
		}

		expressao := texto[inicio+2 : inicio+fim]
		saida.WriteString(avaliarExpressao(expressao, ctx))

		texto = texto[inicio+fim+2:]
	}

	return saida.String()
}

// avalia filtros e resolve o valor final
func avaliarExpressao(expr string, ctx map[string]any) string {
	partes := strings.Split(expr, "|")
	valor := resolverValor(partes[0], ctx)

	for _, filtro := range partes[1:] {
		switch strings.ToLower(strings.TrimSpace(filtro)) {
		case "upper":
			valor = strings.ToUpper(valor)
		case "lower":
			valor = strings.ToLower(valor)
		case "trim":
			valor = strings.TrimSpace(valor)
		}
	}

	return valor
}

// resolve user.name usando reflection
func resolverValor(caminho string, ctx map[string]any) string {
	segmentos := strings.Split(strings.TrimSpace(caminho), ".")
	atual, existe := ctx[segmentos[0]]
	if !existe {
		return ""
	}

	valor := reflect.ValueOf(atual)
	for _, campo := range segmentos[1:] {
		if valor.Kind() == reflect.Pointer {
			valor = valor.Elem()
		}
		if valor.Kind() != reflect.Struct {
			return ""
		}

		f := valor.FieldByNameFunc(func(n string) bool {
			return normalizar(n) == normalizar(campo)
		})

		if !f.IsValid() {
			return ""
		}

		valor = f
	}

	return fmt.Sprint(valor.Interface())
}

// normaliza nomes para comparação
func normalizar(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

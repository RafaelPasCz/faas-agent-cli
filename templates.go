package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	templatesDir = "templates"
	functionsDir = "functions"
)

//go:embed all:templates
var embeddedTemplates embed.FS

func cliNew(args []string, stdout io.Writer) error {
	var (
		templateName string
		functionName string
	)

	fsFlag := newSubcommandFlagSet("new", "faas-cli new [<template> <funcao>] [flags]", stdout)
	fsFlag.StringVar(&templateName, "template", "", "Nome do template da função")
	fsFlag.StringVar(&functionName, "name", "", "Nome da função")

	pos, err := parseFlagsFlexible(fsFlag, args)
	if err != nil {
		return err
	}

	posIdx := 0
	if templateName == "" && posIdx < len(pos) {
		templateName = pos[posIdx]
		posIdx++
	}
	if functionName == "" && posIdx < len(pos) {
		functionName = pos[posIdx]
		posIdx++
	}
	if posIdx < len(pos) {
		return fmt.Errorf("argumentos extras inesperados para 'new': %s", strings.Join(pos[posIdx:], " "))
	}

	if templateName == "" || functionName == "" {
		available := listAvailableTemplates()
		if len(available) > 0 {
			return fmt.Errorf("informe <template> e <funcao> (ex.: faas-cli new %s minha-funcao; disponíveis: %s)", available[0], strings.Join(available, ", "))
		}
		return fmt.Errorf("informe <template> e <funcao> (ex.: faas-cli new python3-http minha-funcao)")
	}

	if err := scaffoldFunction(templateName, functionName); err != nil {
		return err
	}

	destDir := filepath.Join(functionsDir, functionName)
	fmt.Fprintf(stdout, "Função '%s' criada em '%s/' a partir do template '%s'.\n", functionName, destDir, templateName)
	fmt.Fprintf(stdout, "Próximos passos:\n")
	fmt.Fprintf(stdout, "  1. Edite o código em %s/function/handler.py\n", destDir)
	fmt.Fprintf(stdout, "  2. Gere a imagem Docker:\n")
	fmt.Fprintf(stdout, "     docker build -t %s:latest ./%s\n", functionName, destDir)
	fmt.Fprintf(stdout, "  3. Faça o deploy no gateway faas-agent:\n")
	fmt.Fprintf(stdout, "     faas-cli deploy %s %s:latest\n", functionName, functionName)
	return nil
}

func cliTemplates(args []string, stdout io.Writer) error {
	var jsonOutput bool

	fsFlag := newSubcommandFlagSet("templates", "faas-cli templates [--json]", stdout)
	fsFlag.BoolVar(&jsonOutput, "json", false, "Exibe os templates disponíveis em JSON")

	pos, err := parseFlagsFlexible(fsFlag, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf("argumentos inesperados para 'templates': %s", strings.Join(pos, " "))
	}

	names := listAvailableTemplates()
	if jsonOutput {
		raw, err := json.Marshal(names)
		if err != nil {
			return fmt.Errorf("erro ao serializar templates: %w", err)
		}
		fmt.Fprintln(stdout, string(raw))
		return nil
	}

	printAvailableTemplates(stdout, names)
	return nil
}

func printAvailableTemplates(out io.Writer, names []string) {
	if len(names) == 0 {
		fmt.Fprintln(out, "Nenhum template disponível.")
		return
	}
	fmt.Fprintln(out, "Templates disponíveis:")
	for _, name := range names {
		fmt.Fprintf(out, "  - %s\n", name)
	}
}

func listAvailableTemplates() []string {
	seen := make(map[string]bool)

	if entries, err := os.ReadDir(templatesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				seen[entry.Name()] = true
			}
		}
	}

	if entries, err := fs.ReadDir(embeddedTemplates, templatesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				seen[entry.Name()] = true
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func resolveTemplateFS(templateName string) (fs.FS, error) {
	if !isSafeSegment(templateName) {
		return nil, fmt.Errorf("nome de template inválido: '%s'", templateName)
	}

	localPath := filepath.Join(templatesDir, templateName)
	if info, err := os.Stat(localPath); err == nil && info.IsDir() {
		return os.DirFS(localPath), nil
	}

	embeddedPath := templatesDir + "/" + templateName
	if _, err := fs.ReadDir(embeddedTemplates, embeddedPath); err == nil {
		return fs.Sub(embeddedTemplates, embeddedPath)
	}

	available := listAvailableTemplates()
	if len(available) > 0 {
		return nil, fmt.Errorf("template '%s' não encontrado (disponíveis: %s)", templateName, strings.Join(available, ", "))
	}
	return nil, fmt.Errorf("template '%s' não encontrado", templateName)
}

func scaffoldFunction(templateName, functionName string) error {
	if !isSafeSegment(functionName) {
		return fmt.Errorf("nome de função inválido: '%s' (não use espaços, '/' ou '..')", functionName)
	}

	srcFS, err := resolveTemplateFS(templateName)
	if err != nil {
		return err
	}

	destDir := filepath.Join(functionsDir, functionName)
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("o diretório '%s' já existe; remova-o ou escolha outro nome", destDir)
	}

	if err := os.MkdirAll(functionsDir, 0o755); err != nil {
		return fmt.Errorf("erro ao criar diretório '%s': %w", functionsDir, err)
	}

	if err := os.CopyFS(destDir, srcFS); err != nil {
		return fmt.Errorf("erro ao copiar template '%s' para '%s': %w", templateName, destDir, err)
	}

	return nil
}

func isSafeSegment(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || trimmed != name || trimmed == "." || trimmed == ".." {
		return false
	}
	return !strings.ContainsAny(trimmed, " \t\n\r/\\")
}

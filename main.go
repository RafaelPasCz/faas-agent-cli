package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultGatewayURL = "http://localhost:8081"

var cliHTTPClient = &http.Client{
	Timeout: 5 * time.Minute,
}

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
}

func resolveGatewayURL() string {
	return normalizeGatewayURL(os.Getenv("FAAS_GATEWAY"))
}

func normalizeGatewayURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return defaultGatewayURL
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return "http://" + trimmed
	}
	return trimmed
}

func extractGlobalGateway(args []string) (gatewayOverride string, rest []string, err error) {
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-g" || arg == "--gateway":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("a flag --gateway exige um endereço (ex.: --gateway %s)", defaultGatewayURL)
			}
			val := strings.TrimSpace(args[i+1])
			if val == "" {
				return "", nil, fmt.Errorf("a flag --gateway exige um endereço (ex.: --gateway %s)", defaultGatewayURL)
			}
			gatewayOverride = val
			i += 2
		case strings.HasPrefix(arg, "-g="):
			val := strings.TrimSpace(strings.TrimPrefix(arg, "-g="))
			if val == "" {
				return "", nil, fmt.Errorf("a flag --gateway exige um endereço (ex.: --gateway %s)", defaultGatewayURL)
			}
			gatewayOverride = val
			i++
		case strings.HasPrefix(arg, "--gateway="):
			val := strings.TrimSpace(strings.TrimPrefix(arg, "--gateway="))
			if val == "" {
				return "", nil, fmt.Errorf("a flag --gateway exige um endereço (ex.: --gateway %s)", defaultGatewayURL)
			}
			gatewayOverride = val
			i++
		default:
			return gatewayOverride, args[i:], nil
		}
	}
	return gatewayOverride, nil, nil
}

func runCLI(args []string) error {
	return runCLIWithIO(args, os.Stdin, os.Stdout, os.Stderr)
}

func runCLIWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	globalGateway, rest, err := extractGlobalGateway(args)
	if err != nil {
		return err
	}

	if len(rest) == 0 {
		if globalGateway != "" {
			return fmt.Errorf("informe um subcomando após --gateway (use 'faas-cli help' para listar os comandos)")
		}
		printCLIHelp(stdout)
		return nil
	}

	defaultGateway := resolveGatewayURL()
	if globalGateway != "" {
		defaultGateway = normalizeGatewayURL(globalGateway)
	}

	switch rest[0] {
	case "new":
		err = cliNew(rest[1:], stdout)
	case "templates":
		err = cliTemplates(rest[1:], stdout)
	case "deploy":
		err = cliDeploy(rest[1:], defaultGateway, stdout)
	case "delete", "rm":
		err = cliDelete(rest[1:], defaultGateway, stdout)
	case "invoke":
		err = cliInvoke(rest[1:], defaultGateway, stdin, stdout, stderr)
	case "logs":
		err = cliLogs(rest[1:], defaultGateway, stdout)
	case "functions", "ls":
		err = cliFunctions(rest[1:], defaultGateway, stdout)
	case "help", "-h", "--help":
		printCLIHelp(stdout)
	default:
		fmt.Fprintf(stderr, "Subcomando desconhecido: '%s'\n\n", rest[0])
		printCLIHelp(stderr)
		return fmt.Errorf("subcomando inválido: %s", rest[0])
	}

	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func printCLIHelp(out io.Writer) {
	fmt.Fprintln(out, "Uso:")
	fmt.Fprintln(out, "  faas-cli [opções] <subcomando> [argumentos]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcomandos:")
	fmt.Fprintln(out, "  faas-cli new <template> <funcao>      Cria uma nova função em functions/<funcao>")
	fmt.Fprintln(out, "  faas-cli templates                    Lista os templates disponíveis")
	fmt.Fprintln(out, "  faas-cli deploy <funcao> <imagem>     Implanta uma função no gateway faas-agent")
	fmt.Fprintln(out, "  faas-cli functions [funcao]   (ls)    Lista funções implantadas ou detalha uma função")
	fmt.Fprintln(out, "  faas-cli invoke <funcao> [-d JSON]    Invoca uma função e exibe a resposta")
	fmt.Fprintln(out, "  faas-cli logs [funcao] [-n 20]        Consulta os logs de execução do gateway")
	fmt.Fprintln(out, "  faas-cli delete <funcao>      (rm)    Remove uma função e suas réplicas")
	fmt.Fprintln(out, "  faas-cli help                         Exibe esta ajuda")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Gateway (não persistido entre execuções):")
	fmt.Fprintln(out, "  -g, --gateway <url>   Endereço do faas-agent para este comando (ex.: http://192.168.1.10:8081)")
	fmt.Fprintln(out, "  FAAS_GATEWAY          Variável de ambiente alternativa (padrão: http://localhost:8081)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Use '<subcomando> --help' para ver as flags disponíveis (ex.: 'faas-cli invoke --help').")
}

type boolFlag interface {
	flag.Value
	IsBoolFlag() bool
}

func newSubcommandFlagSet(name, usageLine string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintf(out, "Uso: %s\n", usageLine)
		fs.SetOutput(out)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
	}
	return fs
}

func registerGatewayFlag(fs *flag.FlagSet, target *string, defaultVal string) {
	fs.StringVar(target, "g", defaultVal, "Endereço do gateway faas-agent (também via FAAS_GATEWAY; não persistido)")
	fs.StringVar(target, "gateway", defaultVal, "Endereço do gateway faas-agent (também via FAAS_GATEWAY; não persistido)")
}

// parseFlagsFlexible permite passar flags tanto antes quanto depois dos argumentos posicionais.
func parseFlagsFlexible(fs *flag.FlagSet, args []string) ([]string, error) {
	isBool := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		bf, ok := f.Value.(boolFlag)
		isBool[f.Name] = ok && bf.IsBoolFlag()
	})

	var flagArgs []string
	var positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			trimmed := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
			name, _, hasEq := strings.Cut(trimmed, "=")
			flagArgs = append(flagArgs, arg)
			if !hasEq {
				if boolVal, known := isBool[name]; known && !boolVal && i+1 < len(args) {
					flagArgs = append(flagArgs, args[i+1])
					i++
				}
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	// fs.Parse já invoca fs.Usage() internamente antes de retornar flag.ErrHelp.
	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}

	return append(positionals, fs.Args()...), nil
}

func doGatewayRequest(method, rawURL, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("erro ao montar requisição HTTP: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("não foi possível conectar ao gateway faas-agent em %s (%v). Verifique o endereço informado (--gateway / FAAS_GATEWAY) e se o servidor está rodando", rawURL, err)
	}
	return resp, nil
}

func readHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("gateway retornou HTTP %d: %s", resp.StatusCode, msg)
}

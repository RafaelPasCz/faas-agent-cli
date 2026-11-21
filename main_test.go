package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseFlagsFlexible(t *testing.T) {
	var data string
	var quiet bool
	var limit int

	fs := newSubcommandFlagSet("test", "test", io.Discard)
	fs.StringVar(&data, "d", "", "")
	fs.BoolVar(&quiet, "q", false, "")
	fs.IntVar(&limit, "n", 20, "")

	pos, err := parseFlagsFlexible(fs, []string{"hello-python", "-d", `{"name":"Rafael"}`, "-q", "-n", "5"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(pos) != 1 || pos[0] != "hello-python" {
		t.Fatalf("posicionais esperados [hello-python], obtido %v", pos)
	}
	if data != `{"name":"Rafael"}` {
		t.Fatalf("data inesperado: %q", data)
	}
	if !quiet {
		t.Fatalf("quiet deveria ser true")
	}
	if limit != 5 {
		t.Fatalf("limit esperado 5, obtido %d", limit)
	}
}

func TestParseFlagsFlexibleStdinDashAndDoubleDash(t *testing.T) {
	var data string
	fs := newSubcommandFlagSet("invoke", "invoke", io.Discard)
	fs.StringVar(&data, "d", "", "")

	pos, err := parseFlagsFlexible(fs, []string{"fn-a", "-d", "-", "--", "-arg"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if data != "-" {
		t.Fatalf("esperava data='-', obtido %q", data)
	}
	if len(pos) != 2 || pos[0] != "fn-a" || pos[1] != "-arg" {
		t.Fatalf("posicionais inesperados: %v", pos)
	}
}

func TestNormalizeAndResolveGatewayURL(t *testing.T) {
	t.Setenv("FAAS_GATEWAY", "")
	if got := resolveGatewayURL(); got != defaultGatewayURL {
		t.Fatalf("esperava padrão %q, obtido %q", defaultGatewayURL, got)
	}

	t.Setenv("FAAS_GATEWAY", " 192.168.1.10:8081/ ")
	if got := resolveGatewayURL(); got != "http://192.168.1.10:8081" {
		t.Fatalf("esperava http://192.168.1.10:8081, obtido %q", got)
	}

	cases := []struct {
		in   string
		want string
	}{
		{"", defaultGatewayURL},
		{"   ", defaultGatewayURL},
		{"192.168.1.10:8081", "http://192.168.1.10:8081"},
		{"http://edge-node:8081///", "http://edge-node:8081"},
		{"https://gateway.local:8443/", "https://gateway.local:8443"},
	}
	for _, tc := range cases {
		if got := normalizeGatewayURL(tc.in); got != tc.want {
			t.Errorf("normalizeGatewayURL(%q) = %q, esperava %q", tc.in, got, tc.want)
		}
	}
}

func TestCLIGlobalGatewayFlag(t *testing.T) {
	var hitsA, hitsB int
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA++
		json.NewEncoder(w).Encode([]*FunctionInstance{})
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB++
		json.NewEncoder(w).Encode([]*FunctionInstance{})
	}))
	defer serverB.Close()

	var stdout, stderr bytes.Buffer
	if err := runCLIWithIO([]string{"-g", serverA.URL, "ls"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("-g antes do subcomando falhou: %v", err)
	}
	if hitsA != 1 || hitsB != 0 {
		t.Fatalf("esperava hitsA=1, hitsB=0; obtido hitsA=%d, hitsB=%d", hitsA, hitsB)
	}

	stdout.Reset()
	if err := runCLIWithIO([]string{"--gateway=" + strings.TrimPrefix(serverA.URL, "http://"), "ls"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("--gateway=<host:port> falhou: %v", err)
	}
	if hitsA != 2 || hitsB != 0 {
		t.Fatalf("esperava hitsA=2, hitsB=0; obtido hitsA=%d, hitsB=%d", hitsA, hitsB)
	}

	stdout.Reset()
	if err := runCLIWithIO([]string{"-g", serverA.URL, "ls", "--gateway", serverB.URL}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("override de gateway no subcomando falhou: %v", err)
	}
	if hitsA != 2 || hitsB != 1 {
		t.Fatalf("subcomando deveria sobrescrever gateway global (hitsA=%d, hitsB=%d)", hitsA, hitsB)
	}

	if err := runCLIWithIO([]string{"-g"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatalf("esperava erro quando -g é passado sem endereço")
	}
	if err := runCLIWithIO([]string{"--gateway", serverA.URL}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatalf("esperava erro quando --gateway é passado sem subcomando")
	}
}

func TestCLIDeploy(t *testing.T) {
	var captured DeployRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/deploy" {
			t.Fatalf("requisição inesperada: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("erro decodificando payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(&FunctionInstance{
			FunctionName:       captured.FunctionName,
			ImageName:          captured.ImageName,
			Status:             "RUNNING",
			ExecutionState:     "IDLE",
			IdleTimeoutSeconds: 30,
			MaxReplicas:        5,
			MaxConcurrency:     2,
			RunningReplicas:    1,
			TotalReplicas:      1,
			Replicas: []*FunctionReplica{
				{
					ReplicaName: captured.FunctionName + "-rep-1",
					ContainerID: "abc123456789",
					HostPort:    "32768",
					Status:      "RUNNING",
				},
			},
		})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := runCLIWithIO([]string{
		"deploy", "hello-python", "switchfaas/hello-python:latest",
		"--idle-timeout", "30",
		"--max-replicas", "5",
		"--max-concurrency", "2",
		"--gateway", server.URL,
	}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("deploy falhou: %v", err)
	}

	if captured.FunctionName != "hello-python" || captured.ImageName != "switchfaas/hello-python:latest" {
		t.Fatalf("payload inesperado: %+v", captured)
	}
	if captured.IdleTimeoutSeconds != 30 || captured.MaxReplicas != 5 || captured.MaxConcurrency != 2 {
		t.Fatalf("opções de escala inesperadas: %+v", captured)
	}
	if !strings.Contains(stdout.String(), "Função 'hello-python' implantada com sucesso!") {
		t.Fatalf("saída inesperada:\n%s", stdout.String())
	}
}

func TestCLIFunctionsListAndSingle(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/functions":
			json.NewEncoder(w).Encode([]*FunctionInstance{
				{
					FunctionName:    "hello-python",
					ImageName:       "switchfaas/hello-python:latest",
					Status:          "RUNNING",
					ExecutionState:  "IDLE",
					RunningReplicas: 1,
					TotalReplicas:   1,
					MaxReplicas:     3,
					LastUsedAt:      now,
				},
			})
		case "/function/hello-python":
			json.NewEncoder(w).Encode(&FunctionInstance{
				FunctionName:       "hello-python",
				ImageName:          "switchfaas/hello-python:latest",
				Status:             "RUNNING",
				ExecutionState:     "IDLE",
				RunningReplicas:    1,
				TotalReplicas:      1,
				MaxReplicas:        3,
				MaxConcurrency:     4,
				IdleTimeoutSeconds: 60,
				Replicas: []*FunctionReplica{
					{
						ReplicaName: "hello-python-rep-1",
						ContainerID: "deadbeef1234",
						HostPort:    "32769",
						Status:      "RUNNING",
						LastUsedAt:  now,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if err := runCLIWithIO([]string{"ls", "-g", server.URL}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("ls falhou: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello-python") || !strings.Contains(stdout.String(), "1/1 (max 3)") {
		t.Fatalf("saída inesperada do ls:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := runCLIWithIO([]string{"functions", "hello-python", "--gateway", server.URL}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("functions <nome> falhou: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello-python-rep-1") || !strings.Contains(stdout.String(), "32769") {
		t.Fatalf("saída inesperada de detalhe da função:\n%s", stdout.String())
	}
}

func TestCLIInvokeInlineAndStdin(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/invoke/hello-python" {
			t.Fatalf("requisição inesperada: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		receivedBody = string(raw)

		w.Header().Set("X-Execution-Duration-Ms", "4")
		w.Header().Set("X-Cold-Start", "false")
		w.Header().Set("X-Replica-Name", "hello-python-rep-1")
		w.Write([]byte(`{"message":"Olá, Rafael!"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := runCLIWithIO([]string{"invoke", "hello-python", "-d", `{"name":"Rafael"}`, "--gateway", server.URL}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("invoke inline falhou: %v", err)
	}
	if receivedBody != `{"name":"Rafael"}` {
		t.Fatalf("body enviado inesperado: %q", receivedBody)
	}
	if !strings.Contains(stdout.String(), `{"message":"Olá, Rafael!"}`) {
		t.Fatalf("stdout inesperado: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Réplica: hello-python-rep-1") {
		t.Fatalf("stderr inesperado: %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	err = runCLIWithIO([]string{"invoke", "hello-python", "-d", "-", "-q", "-g", server.URL}, strings.NewReader(`{"from":"stdin"}`), &stdout, &stderr)
	if err != nil {
		t.Fatalf("invoke stdin falhou: %v", err)
	}
	if receivedBody != `{"from":"stdin"}` {
		t.Fatalf("stdin body inesperado: %q", receivedBody)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr deveria estar vazio com -q, obtido: %q", stderr.String())
	}
}

func TestCLILogsAndDelete(t *testing.T) {
	var seenQuery string
	var deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/logs":
			seenQuery = r.URL.RawQuery
			json.NewEncoder(w).Encode([]ExecutionLog{
				{
					ID:                7,
					FunctionName:      "hello-python",
					ReplicaName:       "hello-python-rep-1",
					StatusCode:        200,
					RequestDurationMs: 5,
					Success:           true,
					ColdStart:         false,
					ExecutedAt:        time.Date(2026, 11, 20, 21, 0, 0, 0, time.Local),
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/function/hello-python":
			deletedPath = r.URL.Path
			json.NewEncoder(w).Encode(map[string]any{
				"message":         "Função 'hello-python' e suas 1 réplica(s) removidas com sucesso",
				"functionName":    "hello-python",
				"removedReplicas": 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := runCLIWithIO([]string{"logs", "hello-python", "-n", "10", "--after-id", "5", "--gateway", server.URL}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("logs falhou: %v", err)
	}
	if !strings.Contains(seenQuery, "functionName=hello-python") || !strings.Contains(seenQuery, "limit=10") || !strings.Contains(seenQuery, "afterId=5") {
		t.Fatalf("query inesperada em /logs: %q", seenQuery)
	}
	if !strings.Contains(stdout.String(), "hello-python-rep-1") || !strings.Contains(stdout.String(), "5ms") {
		t.Fatalf("tabela de logs inesperada:\n%s", stdout.String())
	}

	stdout.Reset()
	err = runCLIWithIO([]string{"rm", "hello-python", "-g", server.URL}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("delete/rm falhou: %v", err)
	}
	if deletedPath != "/function/hello-python" {
		t.Fatalf("path DELETE inesperado: %q", deletedPath)
	}
	if !strings.Contains(stdout.String(), "removidas com sucesso") {
		t.Fatalf("saída inesperada do delete:\n%s", stdout.String())
	}
}

func TestCLITemplates(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runCLIWithIO([]string{"templates"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("templates retornou erro: %v", err)
	}
	if !strings.Contains(stdout.String(), "python3-http") {
		t.Fatalf("saída de templates deveria conter python3-http, veio:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runCLIWithIO([]string{"templates", "--json"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("templates --json retornou erro: %v", err)
	}
	var listed []string
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("templates --json inválido (%v): %s", err, stdout.String())
	}
	foundPython := false
	for _, item := range listed {
		if item == "python3-http" {
			foundPython = true
			break
		}
	}
	if !foundPython {
		t.Fatalf("templates --json deveria conter python3-http, veio: %v", listed)
	}

	stdout.Reset()
	stderr.Reset()
	if err := runCLIWithIO([]string{"new", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("new --help retornou erro: %v", err)
	}
	if !strings.Contains(stdout.String(), "-template") || !strings.Contains(stdout.String(), "-name") {
		t.Fatalf("new --help deveria documentar flags -template e -name, veio:\n%s", stdout.String())
	}
	if strings.Count(stdout.String(), "Uso:") != 1 {
		t.Fatalf("new --help deveria exibir Uso: uma única vez, veio:\n%s", stdout.String())
	}

	t.Chdir(t.TempDir())
	stdout.Reset()
	stderr.Reset()
	if err := runCLIWithIO([]string{"new", "--template", "python3-http", "--name", "demo-fn"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("new com flags retornou erro: %v", err)
	}
	handlerPath := filepath.Join(functionsDir, "demo-fn", "function", "handler.py")
	if _, err := os.Stat(handlerPath); err != nil {
		t.Fatalf("esperava arquivo gerado em %s: %v", handlerPath, err)
	}
}

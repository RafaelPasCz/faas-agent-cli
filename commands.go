package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
)

func cliDeploy(args []string, defaultGateway string, stdout io.Writer) error {
	var (
		functionName   string
		imageName      string
		idleTimeout    int
		maxReplicas    int
		maxConcurrency int
		gatewayURL     string
		jsonOutput     bool
	)

	fs := newSubcommandFlagSet("deploy", "faas-cli deploy <funcao> <imagem> [flags]", stdout)
	fs.StringVar(&functionName, "name", "", "Nome da função")
	fs.StringVar(&imageName, "image", "", "Imagem Docker da função")
	fs.IntVar(&idleTimeout, "idle-timeout", 0, "Tempo limite de ociosidade em segundos (padrão do agente: 60)")
	fs.IntVar(&maxReplicas, "max-replicas", 0, "Número máximo de réplicas (padrão do agente: 3)")
	fs.IntVar(&maxConcurrency, "max-concurrency", 0, "Concorrência por réplica antes de escalar (padrão do agente: 4)")
	registerGatewayFlag(fs, &gatewayURL, defaultGateway)
	fs.BoolVar(&jsonOutput, "json", false, "Exibe a resposta bruta em JSON")

	pos, err := parseFlagsFlexible(fs, args)
	if err != nil {
		return err
	}

	posIdx := 0
	if functionName == "" && posIdx < len(pos) {
		functionName = pos[posIdx]
		posIdx++
	}
	if imageName == "" && posIdx < len(pos) {
		imageName = pos[posIdx]
		posIdx++
	}
	if posIdx < len(pos) {
		return fmt.Errorf("argumentos extras inesperados para 'deploy': %s", strings.Join(pos[posIdx:], " "))
	}

	if functionName == "" || imageName == "" {
		return fmt.Errorf("informe <funcao> e <imagem> (ex.: faas-cli deploy hello-python switchfaas/hello-python:latest)")
	}

	payload := DeployRequest{
		FunctionName:       functionName,
		ImageName:          imageName,
		IdleTimeoutSeconds: idleTimeout,
		MaxReplicas:        maxReplicas,
		MaxConcurrency:     maxConcurrency,
	}

	rawBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar requisição: %w", err)
	}

	targetURL := normalizeGatewayURL(gatewayURL) + "/deploy"
	resp, err := doGatewayRequest(http.MethodPost, targetURL, "application/json", bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return readHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta do deploy: %w", err)
	}

	if jsonOutput {
		fmt.Fprintln(stdout, strings.TrimSpace(string(body)))
		return nil
	}

	var fn FunctionInstance
	if err := json.Unmarshal(body, &fn); err != nil {
		fmt.Fprintln(stdout, strings.TrimSpace(string(body)))
		return nil
	}

	fmt.Fprintf(stdout, "Função '%s' implantada com sucesso!\n", fn.FunctionName)
	fmt.Fprintf(stdout, "  Imagem:      %s\n", fn.ImageName)
	fmt.Fprintf(stdout, "  Status:      %s (%s)\n", fn.Status, fn.ExecutionState)
	fmt.Fprintf(stdout, "  Escala:      maxReplicas=%d | maxConcurrency=%d | idleTimeout=%ds\n",
		fn.MaxReplicas, fn.MaxConcurrency, fn.IdleTimeoutSeconds)
	for _, rep := range fn.Replicas {
		fmt.Fprintf(stdout, "  Réplica:     %s (127.0.0.1:%s | container=%s | status=%s)\n",
			rep.ReplicaName, rep.HostPort, rep.ContainerID, rep.Status)
	}
	return nil
}

func cliDelete(args []string, defaultGateway string, stdout io.Writer) error {
	var (
		gatewayURL string
		jsonOutput bool
	)

	fs := newSubcommandFlagSet("delete", "faas-cli delete <funcao> [flags]", stdout)
	registerGatewayFlag(fs, &gatewayURL, defaultGateway)
	fs.BoolVar(&jsonOutput, "json", false, "Exibe a resposta bruta em JSON")

	pos, err := parseFlagsFlexible(fs, args)
	if err != nil {
		return err
	}

	if len(pos) != 1 {
		return fmt.Errorf("informe exatamente o nome da função (ex.: faas-cli delete hello-python)")
	}

	functionName := pos[0]
	targetURL := normalizeGatewayURL(gatewayURL) + "/function/" + url.PathEscape(functionName)
	resp, err := doGatewayRequest(http.MethodDelete, targetURL, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return readHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta da remoção: %w", err)
	}

	if jsonOutput {
		fmt.Fprintln(stdout, strings.TrimSpace(string(body)))
		return nil
	}

	var result struct {
		Message         string `json:"message"`
		FunctionName    string `json:"functionName"`
		RemovedReplicas int    `json:"removedReplicas"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Message != "" {
		fmt.Fprintln(stdout, result.Message)
		return nil
	}

	fmt.Fprintf(stdout, "Função '%s' removida com sucesso.\n", functionName)
	return nil
}

func cliInvoke(args []string, defaultGateway string, stdin io.Reader, stdout, stderr io.Writer) error {
	var (
		data        string
		contentType string
		gatewayURL  string
		quiet       bool
	)

	fs := newSubcommandFlagSet("invoke", "faas-cli invoke <funcao> [-d JSON|-] [flags]", stdout)
	fs.StringVar(&data, "d", "", "Corpo da requisição (ou '-' para ler do stdin)")
	fs.StringVar(&data, "data", "", "Corpo da requisição (ou '-' para ler do stdin)")
	fs.StringVar(&contentType, "content-type", "application/json", "Cabeçalho Content-Type da requisição")
	registerGatewayFlag(fs, &gatewayURL, defaultGateway)
	fs.BoolVar(&quiet, "q", false, "Oculta a linha de telemetria no stderr")
	fs.BoolVar(&quiet, "quiet", false, "Oculta a linha de telemetria no stderr")

	pos, err := parseFlagsFlexible(fs, args)
	if err != nil {
		return err
	}

	if len(pos) != 1 {
		return fmt.Errorf("informe exatamente o nome da função (ex.: faas-cli invoke hello-python -d '{\"name\":\"Rafael\"}')")
	}

	functionName := pos[0]
	var reqBody io.Reader
	if data == "-" {
		rawStdin, readErr := io.ReadAll(stdin)
		if readErr != nil {
			return fmt.Errorf("erro ao ler payload do stdin: %w", readErr)
		}
		reqBody = bytes.NewReader(rawStdin)
	} else if data != "" {
		reqBody = strings.NewReader(data)
	} else {
		reqBody = http.NoBody
	}

	targetURL := normalizeGatewayURL(gatewayURL) + "/invoke/" + url.PathEscape(functionName)
	resp, err := doGatewayRequest(http.MethodPost, targetURL, contentType, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	replicaName := resp.Header.Get("X-Replica-Name")
	if resp.StatusCode >= 400 && replicaName == "" {
		return readHTTPError(resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta da função: %w", err)
	}

	if len(respBody) > 0 {
		outStr := string(respBody)
		if !strings.HasSuffix(outStr, "\n") {
			outStr += "\n"
		}
		fmt.Fprint(stdout, outStr)
	}

	if !quiet {
		duration := resp.Header.Get("X-Execution-Duration-Ms")
		coldStart := resp.Header.Get("X-Cold-Start")
		fmt.Fprintf(stderr, "-> [Invoke] Status: %d | Réplica: %s | Cold Start: %s | Duração: %sms\n",
			resp.StatusCode, replicaName, coldStart, duration)
	}

	return nil
}

func cliLogs(args []string, defaultGateway string, stdout io.Writer) error {
	var (
		functionName string
		limit        int
		afterID      int64
		gatewayURL   string
		jsonOutput   bool
	)

	fs := newSubcommandFlagSet("logs", "faas-cli logs [funcao] [flags]", stdout)
	fs.StringVar(&functionName, "f", "", "Filtra pelo nome da função")
	fs.StringVar(&functionName, "function", "", "Filtra pelo nome da função")
	fs.IntVar(&limit, "n", 20, "Quantidade máxima de registros (0 para todos)")
	fs.IntVar(&limit, "limit", 20, "Quantidade máxima de registros (0 para todos)")
	fs.Int64Var(&afterID, "after-id", -1, "Retorna apenas logs com id > after-id")
	registerGatewayFlag(fs, &gatewayURL, defaultGateway)
	fs.BoolVar(&jsonOutput, "json", false, "Exibe a resposta bruta em JSON")

	pos, err := parseFlagsFlexible(fs, args)
	if err != nil {
		return err
	}

	if functionName == "" && len(pos) > 0 {
		functionName = pos[0]
	}

	q := url.Values{}
	if functionName != "" {
		q.Set("functionName", functionName)
	}
	if afterID >= 0 {
		q.Set("afterId", strconv.FormatInt(afterID, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	targetURL := normalizeGatewayURL(gatewayURL) + "/logs"
	if encoded := q.Encode(); encoded != "" {
		targetURL += "?" + encoded
	}

	resp, err := doGatewayRequest(http.MethodGet, targetURL, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return readHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler logs: %w", err)
	}

	if jsonOutput {
		fmt.Fprintln(stdout, strings.TrimSpace(string(body)))
		return nil
	}

	var entries []ExecutionLog
	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("erro ao decodificar logs JSON: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintln(stdout, "Nenhum log de execução encontrado.")
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tDATA/HORA\tFUNÇÃO\tRÉPLICA\tSTATUS\tRESULTADO\tCOLD\tDURAÇÃO\tERRO")
	for _, entry := range entries {
		res := "FALHA"
		if entry.Success {
			res = "OK"
		}
		cold := "não"
		if entry.ColdStart {
			cold = "sim"
		}
		replica := entry.ReplicaName
		if replica == "" {
			replica = "-"
		}
		errText := entry.ErrorMessage
		if errText == "" {
			errText = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%s\t%s\t%dms\t%s\n",
			entry.ID,
			entry.ExecutedAt.Local().Format("02/01/2006 15:04:05"),
			entry.FunctionName,
			replica,
			entry.StatusCode,
			res,
			cold,
			entry.RequestDurationMs,
			errText,
		)
	}
	return tw.Flush()
}

func cliFunctions(args []string, defaultGateway string, stdout io.Writer) error {
	var (
		gatewayURL string
		jsonOutput bool
	)

	fs := newSubcommandFlagSet("functions", "faas-cli functions [funcao] [flags]", stdout)
	registerGatewayFlag(fs, &gatewayURL, defaultGateway)
	fs.BoolVar(&jsonOutput, "json", false, "Exibe a resposta bruta em JSON")

	pos, err := parseFlagsFlexible(fs, args)
	if err != nil {
		return err
	}

	if len(pos) > 1 {
		return fmt.Errorf("uso: faas-cli functions [funcao]")
	}

	baseURL := normalizeGatewayURL(gatewayURL)
	if len(pos) == 1 {
		return printSingleFunction(baseURL, pos[0], jsonOutput, stdout)
	}

	resp, err := doGatewayRequest(http.MethodGet, baseURL+"/functions", "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return readHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler lista de funções: %w", err)
	}

	if jsonOutput {
		fmt.Fprintln(stdout, strings.TrimSpace(string(body)))
		return nil
	}

	var functions []*FunctionInstance
	if err := json.Unmarshal(body, &functions); err != nil {
		return fmt.Errorf("erro ao decodificar funções JSON: %w", err)
	}

	if len(functions) == 0 {
		fmt.Fprintln(stdout, "Nenhuma função implantada no gateway.")
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FUNÇÃO\tIMAGEM\tSTATUS\tESTADO\tRÉPLICAS\tATIVAS\tÚLTIMO USO")
	for _, fn := range functions {
		lastUsed := "-"
		if !fn.LastUsedAt.IsZero() {
			lastUsed = fn.LastUsedAt.Local().Format("02/01/2006 15:04:05")
		}
		replicasInfo := fmt.Sprintf("%d/%d (max %d)", fn.RunningReplicas, fn.TotalReplicas, fn.MaxReplicas)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			fn.FunctionName,
			fn.ImageName,
			fn.Status,
			fn.ExecutionState,
			replicasInfo,
			fn.ActiveRequests,
			lastUsed,
		)
	}
	return tw.Flush()
}

func printSingleFunction(baseURL, functionName string, jsonOutput bool, stdout io.Writer) error {
	targetURL := baseURL + "/function/" + url.PathEscape(functionName)
	resp, err := doGatewayRequest(http.MethodGet, targetURL, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return readHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler detalhes da função: %w", err)
	}

	if jsonOutput {
		fmt.Fprintln(stdout, strings.TrimSpace(string(body)))
		return nil
	}

	var fn FunctionInstance
	if err := json.Unmarshal(body, &fn); err != nil {
		return fmt.Errorf("erro ao decodificar função JSON: %w", err)
	}

	fmt.Fprintf(stdout, "Função: %s (%s)\n", fn.FunctionName, fn.ImageName)
	fmt.Fprintf(stdout, "Status: %s | Estado: %s | Ativas: %d | Réplicas: %d/%d (maxReplicas=%d, maxConcurrency=%d, idleTimeout=%ds)\n",
		fn.Status, fn.ExecutionState, fn.ActiveRequests, fn.RunningReplicas, fn.TotalReplicas,
		fn.MaxReplicas, fn.MaxConcurrency, fn.IdleTimeoutSeconds)

	if len(fn.Replicas) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RÉPLICA\tCONTAINER\tPORTA\tSTATUS\tATIVAS\tÚLTIMO USO")
	for _, rep := range fn.Replicas {
		port := rep.HostPort
		if port == "" {
			port = "-"
		}
		lastUsed := "-"
		if !rep.LastUsedAt.IsZero() {
			lastUsed = rep.LastUsedAt.Local().Format("02/01/2006 15:04:05")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
			rep.ReplicaName,
			rep.ContainerID,
			port,
			rep.Status,
			rep.ActiveRequests,
			lastUsed,
		)
	}
	return tw.Flush()
}

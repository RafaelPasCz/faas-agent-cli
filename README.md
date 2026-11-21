# FaaS CLI

Interface de linha de comando (**CLI**) independente do ecossistema **FaaSwitch** para criação local de funções a partir de templates e operação de instâncias `faas-agent` via HTTP.

Escrita 100% com a biblioteca padrão do Go (sem dependências externas), a CLI se comunica diretamente com a API HTTP do `faas-agent`, preservando a filosofia *standalone* do agente em relação ao `faas-manager`.

---

## Compilação e Execução

```bash
cd faas-cli

# Compilar o binário
go build -o faas-cli .

# Ou executar diretamente via Go
go run . help
```

---

## Configuração do Gateway (sem persistência)

O endereço do gateway (`faas-agent`) **não é persistido** em arquivo. Para cada execução, a resolução segue a ordem abaixo:

1. Flag no subcomando ou antes dele: `--gateway <url>` ou `-g <url>` (se omitido o esquema, `http://` é adicionado automaticamente; ex.: `-g 192.168.1.10:8081`).
2. Variável de ambiente `FAAS_GATEWAY`.
3. Padrão: `http://localhost:8081`.

Exemplos:

```bash
# Flag global antes do subcomando
faas-cli -g http://192.168.1.10:8081 ls

# Flag no próprio subcomando
faas-cli deploy hello-python switchfaas/hello-python:latest --gateway 192.168.1.10:8081

# Variável de ambiente na sessão atual do terminal
export FAAS_GATEWAY=http://192.168.1.10:8081
faas-cli functions
```

---

## Subcomandos Disponíveis

```bash
# 1. Listar templates embutidos/locais e criar uma nova função em functions/<nome>
faas-cli templates
faas-cli templates --json
faas-cli new python3-http hello-python

# 2. Construir a imagem Docker da função e implantar no gateway
docker build -t hello-python:latest ./functions/hello-python
faas-cli deploy hello-python hello-python:latest --max-replicas 3 --max-concurrency 4

# 3. Listar funções implantadas ou detalhar as réplicas de uma função
faas-cli functions        # ou: faas-cli ls
faas-cli functions hello-python

# 4. Invocar uma função (payload inline ou via stdin com '-d -')
faas-cli invoke hello-python -d '{"name": "Rafael"}'
echo '{"name": "Rafael"}' | faas-cli invoke hello-python -d -

# 5. Consultar logs de execução do agente
faas-cli logs
faas-cli logs hello-python -n 10 --after-id 5

# 6. Remover uma função e suas réplicas
faas-cli delete hello-python   # ou: faas-cli rm hello-python
```

Use `faas-cli <subcomando> --help` para consultar todas as flags disponíveis.

---

## Estrutura dos Arquivos

```text
faas-cli/
├── go.mod         # Módulo Go sem dependências externas
├── main.go        # Entrypoint, resolução de gateway (--gateway / FAAS_GATEWAY) e cliente HTTP
├── commands.go    # Subcomandos operacionais (deploy, functions/ls, invoke, logs, delete/rm)
├── templates.go   # Templates embutidos (//go:embed) e subcomandos new / templates
├── models.go      # DTOs JSON compatíveis com a API HTTP do faas-agent
├── main_test.go   # Testes unitários da CLI, resolução de gateway e templates
├── templates/     # Templates base de funções (ex.: python3-http)
└── README.md      # Documentação de uso da CLI
```

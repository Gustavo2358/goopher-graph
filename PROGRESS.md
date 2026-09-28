# Progresso

Estado: implementação incremental iniciada; CLI de ajuda e harness reais disponíveis.

Próxima fatia: **B05**.

| Fatia | Estado | Evidência / próximo passo |
|---|---|---|
| B00 | Concluído | Go global 1.26.0; harness falho retornou 1; `go test ./...`, `go vet ./...`, build sem cgo e `--help` passaram. |
| B01 | Concluído | Grafo/CSR, valores e sets reais; testes de direção, paralelas, loops, alias, limites, Close e zero alocações na adjacência; regressão/vet/build sem cgo passaram. |
| B02 | Concluído | Reachable/territory/anti/between e subgrafo completo; closure independente em 40 grafos, filtros, ciclos, paralelas e cancelamento em hub de 50 mil edges; regressão/vet passaram. |
| B03 | Concluído | Decoder streaming com limites, presença/quoted empty, CRLF, tipos/arrays, rejeições e seeds de fuzz; fragmentação 1/2/7/4096, n>0+erro e records >64 KiB; regressão/vet passaram. |
| B04 | Concluído | As 12 fixtures, modelos, consultas, diagnósticos e contadores passaram; barreira nodes/edges, falhas de ports, close por fonte, permutações e idempotência; regressão/vet/build passaram. |
| B05 | Pendente | — |
| B06 | Pendente | — |
| B07 | Pendente | — |
| B08 | Pendente | — |
| B09 | Pendente | — |
| B10 | Pendente | — |
| B11 | Pendente | — |
| B12 | Pendente | — |

## Última sessão de implementação

2026-09-28: Go 1.26.0 instalado globalmente pelo usuário e conferido. B00 concluída com teste de ajuda em diretório vazio, falha de output e uso inválido. O teste temporário intencionalmente falho falhou e foi removido; regressão e build passaram. Git local inicializado a pedido do usuário; um commit por checkpoint concluído.

B01 concluída: `go test ./graph/... ./internal/graphdata/...`, `go test ./...`, `go vet ./...` e build sem cgo passaram. B02 concluída: `go test ./...` e `go vet ./...` passaram. B03 concluída: testes focais, regressão e vet passaram. B04 concluída: `go test ./...`, vet e build sem cgo passaram. Próximo passo: B05, catálogos locais e diagnóstico stderr.

## Notas para retomar

Ler AGENTS e o próximo cartão liberado. Atualizar este arquivo, sem replicar estado no backlog ou gerar um relatório por sessão. Estados: Pendente, Em andamento, Concluído, Bloqueado (com causa concreta).

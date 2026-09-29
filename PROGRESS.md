# Progresso

Estado: implementação incremental iniciada; CLI de ajuda e harness reais disponíveis.

Próxima fatia: **B08**.

| Fatia | Estado | Evidência / próximo passo |
|---|---|---|
| B00 | Concluído | Go global 1.26.0; harness falho retornou 1; `go test ./...`, `go vet ./...`, build sem cgo e `--help` passaram. |
| B01 | Concluído | Grafo/CSR, valores e sets reais; testes de direção, paralelas, loops, alias, limites, Close e zero alocações na adjacência; regressão/vet/build sem cgo passaram. |
| B02 | Concluído | Reachable/territory/anti/between e subgrafo completo; closure independente em 40 grafos, filtros, ciclos, paralelas e cancelamento em hub de 50 mil edges; regressão/vet passaram. |
| B03 | Concluído | Decoder streaming com limites, presença/quoted empty, CRLF, tipos/arrays, rejeições e seeds de fuzz; fragmentação 1/2/7/4096, n>0+erro e records >64 KiB; regressão/vet passaram. |
| B04 | Concluído | As 12 fixtures, modelos, consultas, diagnósticos e contadores passaram; barreira nodes/edges, falhas de ports, close por fonte, permutações e idempotência; regressão/vet/build passaram. |
| B05 | Concluído | Catálogos Linux sem filtro de extensão, symlinks/diretórios rejeitados, permissões reais e diagnóstico JSON escapado; 12 fixtures em disco, regressão e vet passaram. |
| B06 | Concluído | Postings de labels/propriedades e índice vazio persistido nas colunas; índice/scan equivalentes em todas as fixtures; validação de memberships falsos/faltantes/duplicados; regressão/vet passaram. |
| B07 | Concluído | Codec LE de 24 seções; seis goldens exatos, reader Python independente, truncamentos/mutações, short writes, ownership/abort e seeds de fuzz; regressão/vet passaram. |
| B08 | Pendente | — |
| B09 | Pendente | — |
| B10 | Pendente | — |
| B11 | Pendente | — |
| B12 | Pendente | — |

## Última sessão de implementação

2026-09-28: Go 1.26.0 instalado globalmente pelo usuário e conferido. B00 concluída com teste de ajuda em diretório vazio, falha de output e uso inválido. O teste temporário intencionalmente falho falhou e foi removido; regressão e build passaram. Git local inicializado a pedido do usuário; um commit por checkpoint concluído.

B01 concluída: `go test ./graph/... ./internal/graphdata/...`, `go test ./...`, `go vet ./...` e build sem cgo passaram. B02 concluída: `go test ./...` e `go vet ./...` passaram. B03 concluída: testes focais, regressão e vet passaram. B04 concluída: `go test ./...`, vet e build sem cgo passaram. B05 concluída: catálogos reais produzem os mesmos modelos; testes completos e vet passaram. B06 concluída: testes de graph/ingest, regressão e vet passaram. B07 concluída: goldens exatos e reader independente passaram, assim como regressão e vet. Próximo passo: B08, mmap Linux e publicação atômica.

## Notas para retomar

Ler AGENTS e o próximo cartão liberado. Atualizar este arquivo, sem replicar estado no backlog ou gerar um relatório por sessão. Estados: Pendente, Em andamento, Concluído, Bloqueado (com causa concreta).

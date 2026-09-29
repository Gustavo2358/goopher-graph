# Progresso

Estado: **produto concluído — B00 a B12**.

Próximo passo: **escopo encerrado**. HTTP, S3 e novas consultas são outro escopo.

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
| B08 | Concluído | mmap read-only via x/sys; staging, validação, sync/rename/dir-sync; leitor antigo preservado e falhas com PublishedUncertain; gates snapshot sem cgo, regressão/vet/build passaram. |
| B09 | Concluído | CLI build/territory/anti-territory/between, CSV IDs, DOT e exemplo público; E2E completos/parciais, consumidor em módulo externo offline, Graphviz e imports; regressão/vet/build passaram. |
| B10 | Concluído | Race real com 16 leitores × 50 queries; 100 ciclos open/query/close sem fd/mmap retidos; fuzz CSV 10.010 e snapshot 10.000 execuções; falha do sink/cancelamento/short write testados; regressão/vet passaram. |
| B11 | Concluído | Benchmarks nas três escalas até 100 mil/500 mil; fases/allocs, sete queries, fórmula, expansão única e RSS em processos separados. Resultados em docs/BENCHMARKS.md; regressão/vet passaram. |
| B12 | Concluído | Cópia limpa sem rede: testes sem cache, vet/gofmt, build sem cgo, race, fuzz, benchmarks, goldens, E2E e comandos do README passaram. |

## Fechamento — 2026-09-28

Go 1.26.0 global instalado pelo usuário e conferido; GCC/libc, Python, GNU time e Graphviz disponíveis. x/sys v0.38.0 foi obtido pelo gerenciador normal de módulos. Git local mantém um commit por checkpoint, sem remote ou publicação externa.

Reproduzi o produto em cópia limpa do índice Git, sem binários anteriores e com `GOPROXY=off GOTOOLCHAIN=local`:

- `go clean -testcache` e `go test -count=1 ./...`: passaram, incluindo 12 fixtures, consultas esperadas, seis goldens, reader Python, E2E em subprocessos e consumidor em módulo externo.
- `go vet ./...`, gofmt em todas as fontes e `CGO_ENABLED=0 go build -o bin/gophergraph ./cmd/gophergraph`: passaram.
- `CGO_ENABLED=1 go test -race -count=1 ./...`: passou, incluindo leituras simultâneas e ciclos de recursos.
- As duas campanhas de TESTING com `-fuzztime=10000x`: 10.000 execuções cada, sem falhas.
- `go test -run '^$' -bench . -benchmem ./...`: passou nas três escalas, até 100 mil nodes / 500 mil edges. Fases, alocações, heap, mapping e RSS estão em `docs/BENCHMARKS.md`.
- `python3 tools/check_package.py`: referências e links passaram. Os comandos completos/parciais do README, as três consultas, DOT e o exemplo executável também foram executados com sucesso.

A revisão final acrescentou igualdade de propriedades contra modelos independentes no backend de bytes e escaping de controles Unicode em DOT/diagnósticos. Fixtures e goldens originais foram preservados.

## Limites da evidência

Qualificação local Linux/amd64 com fixtures e dados sintéticos; nenhum corpus corporativo ou cluster Neptune foi fornecido ou acessado. O maior build medido atingiu cerca de 1,41 GiB de RSS; a query CLI correspondente, 56,5 MiB. Nenhuma promessa de SLA ou cold-cache. Leitores precisam terminar antes de Close, e inodes mapeados não podem sofrer writes/truncate externo.

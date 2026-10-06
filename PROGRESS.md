# Progresso

Estado: **produto concluído — B00 a B12**.

Campanhas adicionais **concluídas e incorporadas à `main`**, após aprovação do
usuário em 2026-09-30:

- Ingestão limitada: [PR #1](https://github.com/Gustavo2358/goopher-graph/pull/1).
- Output JSON: [PR #2](https://github.com/Gustavo2358/goopher-graph/pull/2).
- Queries WebAssembly: [PR #3](https://github.com/Gustavo2358/goopher-graph/pull/3).

Fechamento concluído; nenhum próximo passo pendente nesta campanha.
HTTP, registry, S3, autenticação e compilação de source no servidor seguem
fora do escopo.

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

Qualificação local Linux/amd64 com fixtures e dados sintéticos; nenhum corpus corporativo ou cluster Neptune foi fornecido ou acessado. No fechamento B12, o maior build medido atingiu cerca de 1,41 GiB de RSS; a query CLI correspondente, 56,5 MiB. Nenhuma promessa de SLA ou cold-cache. Leitores precisam terminar antes de Close, e inodes mapeados não podem sofrer writes/truncate externo.

## Fechamento — campanha de ingestão limitada

CLI usa ordenação externa, consolidação por streams e colunas mmap; API de
snapshot/queries e formato v1 preservados. `Options.Scratch` habilita o caminho
limitado na biblioteca; `Options{}` mantém compatibilidade com o backend heap.
Orçamento de sort e diretório scratch configuráveis; falhas operacionais não
publicam prefixos. Detalhes e reprodução em [BOUNDED_INGEST](docs/BOUNDED_INGEST.md).

Evidência executada:

- Equivalência byte a byte, reports sem tempos e diagnósticos completos nas 12
  fixtures; modelos/queries independentes, permutações, conflitos entre runs,
  registros maiores que a arena, falhas de scratch e cancelamento passaram.
- `GOPROXY=off GOTOOLCHAIN=local go test -count=1 ./...`, `go vet ./...`, gofmt,
  build `CGO_ENABLED=0`, `CGO_ENABLED=1 go test -race -count=1 ./...`: passaram.
- Fuzz Neptune: 10.002 execuções; snapshot: 10.000. Sem falhas.
- `go test -run '^$' -bench . -benchmem -benchtime=1x ./...`: passou, incluindo
  build, publicação, abertura, alcance, subgrafo e DOT nas três escalas existentes.
- `GOPHERGRAPH_SCALE=1 go test ./ingest -run
  'TestExternal(GeneratedScale|SingleEntityScale)$' -count=1 -v`: passou.
  Com orçamento 1 MiB, 1,2M/2,4M registros tiveram heap máximo 5,58/5,71 MiB;
  240 mil valores set em um ID, 5,25 MiB.
- Medição isolada de 1,2M registros: heap 2.685,95 -> 34,48 MiB;
  RSS 3.198,39 -> 208,45 MiB; build 9,23 -> 16,47 s; snapshots idênticos.
  Em 2,4M registros, heap 34,60 MiB com o mesmo orçamento de 16 MiB.
- Cgroups sem swap: 2,4M registros concluíram sob 256 MiB (sort 16 MiB) e
  64 MiB (sort 1 MiB). Neste último, input ~85 MiB, snapshot ~197 MiB,
  heap 5,42 MiB e build 90,33 s. Nenhum ajuste de GC.
- Cópia limpa de `git archive`: regressão sem cache, vet, build sem cgo e CLI
  completa/parcial passaram. `python3 tools/check_package.py` passou.

A revisão acrescentou regressão para truncamento de scratch exatamente no começo
do payload: deve falhar operacionalmente, nunca aceitar EOF como prefixo válido.
Somente corpus sintético/local foi usado. Limites de record/header, metadados de
catálogo, disco livre e páginas mmap são separados do orçamento de sort.

## Fechamento — output JSON (2026-09-30)

`graphjson.Write(ctx, io.Writer, graph, subgraph, metadata)` exporta o subgrafo
completo por iteradores. CLI aceita `--format json` nas três queries; IDs/DOT,
core e snapshots preservados. [Contrato público](docs/JSON.md).

Evidência em cópia limpa do índice Git, com `GOPROXY=off GOTOOLCHAIN=local`:

- `go test -count=1 ./...`, `go vet ./...`, gofmt e
  `CGO_ENABLED=0 go build -o bin/gophergraph ./cmd/gophergraph`: passaram.
- `CGO_ENABLED=1 go test -race -count=1 ./...`: passou, incluindo writers
  concorrentes, testes grandes, ingestão e E2E.
- Fixtures independentes e casos adicionais cobrem topologia completa, ciclos,
  convergências, paralelas, loops, multilabel, sets, dez tipos, limites int64,
  floats não finitos/zeros assinados, escaping, parcialidade e determinismo.
  E2E cobrem filtros, três comandos, arquivo/stdout, ID vazio e consumidor externo.
- `TestStreamingLargeResult`: 1 mil e 100 mil nodes/edges, limites fixos de
  256 KiB alocados antes da primeira escrita e 8 MiB de heap vivo auxiliar
  amostrado. A escala maior exportou 227.900.170 bytes; delta vivo observado
  17.408 bytes com GC nas amostras. Erro de escrita e cancelamento interrompem
  o fluxo. O teste descarta bytes e não mantém o JSON em memória.
- Medição isolada do binário de teste, compilado antes: RSS máximo 56.564 KiB
  incluindo construção/validação da fixture e query; grafo heap, sem mapping.
  Isso não é uma medição isolada de RSS do serializer nem uma promessa de SLA.
- `python3 tools/check_package.py` e `git diff --check`: passaram.


## Fechamento — queries WebAssembly (2026-09-30)

`wasmquery` adiciona runtime wazero pure Go, Host API v1 por handles e SDK Go
para `wasip1/wasm`. O CLI recebe snapshot, módulo e argumentos; exporta com o
serializer graphjson existente. Cache por SHA-256, instâncias privadas,
limites de memória/tabela/handles/chamadas/tempo/concorrência e cleanup explícito.
[Uso, semântica e limites](docs/WASM.md). Nenhuma mudança em graph, query,
snapshot ou ingestão; x/sys atualizado por requisito do wazero.

Evidência executada com `GOPROXY=off GOTOOLCHAIN=local`:

- Primeiro teste de runtime falhou antes da implementação; depois passaram os
  testes focais, as três queries externas, ABI malformada, imports proibidos,
  isolamento de filesystem/rede/ambiente/stdio, argumentos, handles inválidos,
  stale e de outra execução, release, traps, exit, timeout e Close ativo.
- `go test -count=1 ./...`: passou em cópia limpa do índice e novamente no
  workspace final. Inclui SDK compilado em módulo Go externo offline, CLI real,
  resultado completo/parcial e igualdade determinística com graphjson nativo.
- `CGO_ENABLED=1 go test -race -count=1 ./...`: passou em cópia limpa. Após
  isolar valores de contexto externos (opt-ins WASI), os testes de sandbox e
  cancelamento passaram novamente; o sandbox também passou com `-race`.
- `go vet ./...`, gofmt, `CGO_ENABLED=0 go build`, checker de documentos/goldens
  e `git diff --check`: passaram. O teste de arquitetura também passou no
  workspace, excluindo as cópias de qualificação do diretório ignorado `.measure`.
- Reachable/subgrafo comparados às APIs nativas nas 12 fixtures, em heap e mmap,
  forward/reverse, labels nil/vazios/conhecidos/ausentes. Os dez tipos de property,
  valores set, NaN e zeros assinados foram comparados à API nativa. Concorrência
  com oito execuções, compilação única por conteúdo e lifecycle de resultados
  passaram. Cancelamento também foi exercitado dentro de loops do host.
- Prova de fronteira: 100 e 100.000 nodes (500 e 500.000 edges de entrada) usam
  as mesmas 8 chamadas, 123 bytes de parâmetros e zero bytes de payload de
  resposta da Host API. O resultado maior tem 90.000 nodes e 450.000 edges;
  os handles/contagens são escalares. Bitsets e fila cresceram somente no host.
- Fuzz de admissão WASM: 10.029 execuções; Neptune: 10.001; snapshot: 10.000.
  Sem falhas. O limite de tabela foi testado contra `table.grow`, além do
  limite de memória linear. Nenhuma alteração nas fixtures existentes.

Os limites contabilizados não são teto de RSS; compilação, stacks, grafo e
resultados retidos têm custos separados. Qualificação local Linux/amd64, Go
1.26.0, fixtures e dados sintéticos. HTTP, registry, S3, autenticação, result
cache e compilação de source no servidor permanecem fora do escopo.


## Integração final — 2026-09-30

Aprovação do usuário recebida. PRs #1, #2 e #3 incorporados nessa ordem por
merge commits; #2 e #3 tiveram a base atualizada para `main` após a integração
da dependência. Todos constam como MERGED no GitHub, com os heads aprovados
preservados. Não havia checks de CI configurados nos PRs.

`git diff --exit-code 8e88748 5467f3a` passou: o conteúdo de `main` após os
três merges é idêntico à árvore validada no fechamento WASM acima. As evidências
de regressão, race, vet, build e fuzz continuam aplicáveis; esta atualização
altera somente o registro de progresso. Branch local `main` atualizada por
fast-forward. Campanha encerrada.

## Campanha GGPB — 2026-10-06

Campanha concluída; [PR #4](https://github.com/Gustavo2358/goopher-graph/pull/4)
aberto para revisão, sem merge. Branch `feat/ggpb-streaming-results` publicada.
Adapter externo `ggpb`/`ggpb/pb`, batches Protobuf limitados, framing CRC32,
reader incremental e CLI `--format ggpb` / `decode --format json`, incluindo
WASM (JSON continua padrão, sem mudar runtime/SDK). Única extensão
mínima do core: IterateNodeLabels sem cópia, para limitar memória mesmo em um node
com muitos labels. Queries, sets, snapshot e formatos anteriores preservados.

Evidência executada com dependências locais, GOPROXY=off e Go 1.26.0:

- `go test -count=1 ./...`, `CGO_ENABLED=1 go test -race -count=1 ./...`,
  `go vet ./...`, gofmt, `CGO_ENABLED=0 go build`: passaram.
- Framing/golden, truncamento de todos os prefixos, corrupção, Protobuf inválido,
  versão, ordering/continuidade/counts, erro de I/O, short writes, cancelamento,
  vazios, tipos/extremos, labels e 30 mil properties em partes passaram.
- Paridade JSON byte a byte nos seis goldens e casos adicionais; CLI nas três
  queries, partial, filtros, IDs vazios, stdout/arquivo e proteção de input.
  WASM completo/parcial também passou em GGPB → JSON com metadata idêntica.
- Memória: 214.422.538 bytes descartados em 100 mil nodes/edges, heap auxiliar
  vivo amostrado 365.752 bytes; mesma tolerância fixa nas duas escalas. Não foi
  usado buffer de resultado nem dictionary global. Iterador de labels não aloca.
- Fuzz: GGPB 10.000, Neptune 10.000, snapshot 10.000 e WASM 10.008 execuções;
  sem falhas. `go test -run '^$' -bench . -benchmem -benchtime=1x ./...` passou.
- Campanha real: 300 amostras, três escalas/três queries e caso vazio, producer
  e consumer em processos separados; cinco repetições/format com hashes iguais.
  Territory grande: payload −58,58%, encoding 1,08x mais rápido, decode 2,69x;
  CPU producer +0,6%, RSS 57,30 → 57,94 MiB, TotalAlloc ~120 → ~352 MiB.
  Casos pequenos/vazios e io.Discard grande foram mais lentos em GGPB.
- Conversão real do payload grande comparada por cmp ao JSON direto; três
  emissões de 29.914.323 bytes tiveram mesmo SHA-256. Python oficial Protobuf
  validou arquivo grande e fixtures typed/numeric/presence com os dez tipos.
- `python3 tools/check_package.py` e `git diff --check`: passaram. Nenhuma
  fixture/golden original alterada. GOCACHE em /tmp e TMPDIR em .measure/tmp
  contornam somente as restrições/broken /tmp/.git do ambiente de teste.

[Contrato](docs/GGPB.md), [resultados completos](docs/GGPB_BENCHMARKS.md) e
[amostras brutas](docs/benchmarks/ggpb_samples.jsonl). Cópia limpa de `git archive`
passou offline em graph/GGPB/E2E e build sem cgo, sem binários anteriores.
Próximo: revisão do PR; nenhum merge autorizado nesta entrega.

## Otimização GGPB — 2026-10-06

Em andamento no PR #4, branch existente. Checkpoint 0: capturar CPU/alloc_space/alloc_objects de JSON, GGPB e inline no mesmo snapshot grande antes de alterar o hot path. Próximo: medir cada hipótese isoladamente.

Checkpoint A concluído: StringID dictionary/cache bounded, sem mudar wire; testes ggpb/graph/E2E passaram. Quatro casos cold/resident medidos; grande encode −5,2%, allocations −8,6%. Próximo: endpoints lógicos locais.

Checkpoint B concluído: endpoints locais bounded, schema/reader/JSON/Python atualizados; teste falhou antes, ggpb/E2E e consumidor Python grande passaram. Payload adicional −26,2%, encoding +2,9% justificado pela redução. Próximo: reuso de objetos.

Checkpoint C1 concluído: reuso local de slots e slices, ggpb/E2E/vet passaram; quatro escalas medidas. Encoding grande −29,9%, bytes alocados −90,2%, wire idêntico. Próximo: C2 e proto.Size isolados.

Checkpoint C2 concluído: maps/Records/slices reutilizados, testes ggpb e matriz cold/resident passaram. Grande encoding −5,2%, TotalAlloc −63,8%. Próximo: medir eliminação do Size externo.

Experimento D1 concluído/rejeitado: remover Size externo não ganhou no marshaler gerado (331,5→333,0 ms); revertido. Testes ggpb passaram. Próximo: protowire sobre slots reutilizados.

Checkpoint C3 concluído: protowire específico, EmitEncoded transport-neutral, mesmo wire; ggpb/E2E e diferenciais oficiais passaram. Encoding −21,5%. Próximo: reavaliar Size e batch sizing.

Checkpoint D2 concluído: orçamento conservador + check final de wire, sem Size normal; ggpb passou e quatro casos medidos. Grande 260,3→178,3 ms, CPU 294,5→210,2. Próximo: matriz de batches.

Experimento E concluído: 12 variantes (6 targets × 2 caps) nas quatro escalas. Default 256 partes limita os batches antes do target; 4096 partes piorou encode/alloc/payload. Default preservado. Próximo: dictionary policy e paralelo.

Experimento F concluído: inline e três heurísticas de frequência medidos nas quatro escalas; thresholds 2/4 e símbolos curtos não ganharam e aumentaram payload. Default imediato mantido. Testes ggpb passaram após restauração. Próximo: geometria/live heap, paralelismo e perfis finais.

Experimento H concluído/rejeitado: 192 amostras nas quatro escalas. Cópias de ownership elevaram encoding grande ~166→559+ ms e TotalAlloc ~12→286 MiB/query; 1 worker/8 queries alcançou ~26 queries/s vs ~6 paralelo. Testes de ordem/cancelamento/erro e ggpb/vet passaram. Próximo: residente baseline/final, profiling final e gates completos.

Experimento G concluído/rejeitado: caches cross-batch reduziram pouco CPU/tempo e apenas 0,4% TotalAlloc; revertidos. Perfil aponta IDs externos como principal alocação restante. Próximo: stress de endpoints dispersos antes de consolidar e reexecutar race/fuzz.

# Medições locais

Execução em 2026-09-28, Linux/amd64, Go 1.26.0, AMD Ryzen 5 5600GT, 12 CPUs lógicas. GC padrão, sem pools ou unsafe. As medidas são locais, com caches aquecidos ou sem controle do page cache; não representam cold-cache nem SLA.

A medição histórica abaixo usa o backend heap. A CLI agora usa ingestão externa;
[antes/depois e qualificação de memória](BOUNDED_INGEST.md) medem esse caminho.
O benchmark nativo de Build mantém `Options{}` para medir a API heap compatível.

## Dataset e método

O gerador `tools/benchdata` produz duas componentes desconexas, cadeias, diamantes sobrepostos, ciclos, hubs, loops, paralelas, multilabel e propriedades tipadas. Cada node tem cinco edges. São indexadas as chaves `group` e `score`. O alcance a partir de `n000000000` contém 90% dos nodes e das edges.

A tabela usa a execução final em cópia limpa, com `GOPROXY=off` e `GOTOOLCHAIN=local`. Comando executado:

```sh
go test -run '^$' -bench . -benchmem ./...
```

Os benchmarks verificam contagens e a fórmula de bytes antes de medir. Build separa leitura/merge de canonicalização/CSR/índices em `Report.Times`; os tempos não entram no snapshot. Abrir inclui mmap e validação integral. Consultas usam um Graph já aberto. DOT escreve em `io.Discard`, incluindo formatação e escaping.

| Nodes / edges | Leitura + merge | Canonicalização + CSR + índices | Escrita + validação + commit | Abrir + validar | Alcance | Subgrafo | DOT |
|---|---:|---:|---:|---:|---:|---:|---:|
| 1.000 / 5.000 | 16,48 ms | 9,01 ms | 1,55 ms | 0,74 ms | 0,134 ms | 0,248 ms | 2,36 ms |
| 10.000 / 50.000 | 174,22 ms | 144,06 ms | 15,05 ms | 7,17 ms | 1,345 ms | 2,49 ms | 24,11 ms |
| 100.000 / 500.000 | 2,059 s | 1,860 s | 158,42 ms | 78,52 ms | 13,54 ms | 25,03 ms | 241,72 ms |

Sete alcances adicionais sobre cada Graph aberto mediram mediana e faixa min–max, em ms: **0,135 (0,133–0,166)**; **1,327 (1,321–1,358)**; **13,769 (13,281–14,580)**. A amostra maior do benchmark de build teve uma execução; não inferir precisão estatística desses números.

## Memória e tamanho

| Nodes / edges | Snapshot / bytes mapeados | Heap amostrado após abertura e sete queries | Alcance B/op / allocs/op | Pico RSS build | Pico RSS CLI query |
|---|---:|---:|---:|---:|---:|
| 1.000 / 5.000 | 521.472 B | 483.176 B | 13.097 / 13 | 18,8 MiB | 4,0 MiB |
| 10.000 / 50.000 | 5.165.120 B | 1.548.184 B | 142.516 / 19 | 155,5 MiB | 8,6 MiB |
| 100.000 / 500.000 | 51.605.120 B | 3.535.920 B | 1.981.728 / 28 | 1.439,9 MiB | 56,5 MiB |

Heap é uma amostra no processo de benchmark; inclui resultados temporários das queries. RSS foi medido separadamente com `/usr/bin/time` em processos reais de build e de `territory`, com exportação CSV para arquivo. A query CLI inclui abertura, validação, travessia, subgrafo e exportação. Na maior escala ela levou 0,11 s; o build completo levou 4,33 s. Os três processos de query e os três de build tiveram zero major page faults. Minor faults no build: 4.465, 38.688, 359.254; na query: 442, 598, 2.317.

Reprodução da maior escala, após compilar a CLI:

```sh
go run ./tools/benchdata --nodes 100000 --output /tmp/gophergraph-bench
/usr/bin/time -v ./bin/gophergraph build \
  --nodes /tmp/gophergraph-bench/nodes --edges /tmp/gophergraph-bench/edges \
  --output /tmp/gophergraph-bench/graph.snapshot \
  --index-property group --index-property score
/usr/bin/time -v ./bin/gophergraph territory \
  --snapshot /tmp/gophergraph-bench/graph.snapshot --node n000000000 \
  --output /tmp/gophergraph-bench/ids.csv
```

O build mantém staging com mapas e usa substancialmente mais RAM que a consulta. Não considerar somente o heap Go ao dimensionar leitores mmap. A primeira medição mostrou criação repetida de buffers de validação CSV; reusar o reader da stdlib reduziu a alocação acumulada de build de 4,89 GB para 2,01 GB no dataset maior. Isso é alocação acumulada, não pico de memória residente.

Testes instrumentados confirmam uma expansão por node e uma inspeção por ocorrência de adjacência. Trinta camadas de diamantes não enumeram caminhos. Passar de uma para 10.000 edges paralelas, mantendo os mesmos nodes, não aumenta as alocações da BFS. A abertura percorre o arquivo; ela tem custo próprio e não é O(1).

# Medições locais

Execução em 2026-09-28, Linux/amd64, Go 1.26.0, AMD Ryzen 5 5600GT, 12 CPUs lógicas. GC padrão, sem pools ou unsafe. As medidas são locais, com caches aquecidos ou sem controle do page cache; não representam cold-cache nem SLA.

## Dataset e método

O gerador `tools/benchdata` produz duas componentes desconexas, cadeias, diamantes sobrepostos, ciclos, hubs, loops, paralelas, multilabel e propriedades tipadas. Cada node tem cinco edges. São indexadas as chaves `group` e `score`. O alcance a partir de `n000000000` contém 90% dos nodes e das edges.

Comando executado:

```sh
go test -run '^$' -bench . -benchmem ./...
```

Os benchmarks verificam contagens e a fórmula de bytes antes de medir. Build separa leitura/merge de canonicalização/CSR/índices em `Report.Times`; os tempos não entram no snapshot. Abrir inclui mmap e validação integral. Consultas usam um Graph já aberto. DOT escreve em `io.Discard`, incluindo formatação e escaping.

| Nodes / edges | Leitura + merge | Canonicalização + CSR + índices | Escrita + validação + commit | Abrir + validar | Alcance | Subgrafo | DOT |
|---|---:|---:|---:|---:|---:|---:|---:|
| 1.000 / 5.000 | 16,29 ms | 9,12 ms | 1,55 ms | 0,73 ms | 0,135 ms | 0,252 ms | 2,33 ms |
| 10.000 / 50.000 | 169,44 ms | 139,73 ms | 15,14 ms | 7,17 ms | 1,345 ms | 2,50 ms | 23,79 ms |
| 100.000 / 500.000 | 2,125 s | 1,847 s | 159,54 ms | 78,32 ms | 13,56 ms | 25,14 ms | 241,53 ms |

Sete alcances adicionais sobre cada Graph aberto mediram mediana e faixa min–max, em ms: **0,143 (0,134–0,157)**; **1,345 (1,333–1,375)**; **13,727 (13,449–14,093)**. A amostra maior do benchmark de build teve uma execução; não inferir precisão estatística desses números.

## Memória e tamanho

| Nodes / edges | Snapshot / bytes mapeados | Heap amostrado após abertura e sete queries | Alcance B/op / allocs/op | Pico RSS build | Pico RSS CLI query |
|---|---:|---:|---:|---:|---:|
| 1.000 / 5.000 | 521.472 B | 481.064 B | 13.097 / 13 | 18,8 MiB | 4,0 MiB |
| 10.000 / 50.000 | 5.165.120 B | 1.541.384 B | 142.516 / 19 | 155,5 MiB | 8,6 MiB |
| 100.000 / 500.000 | 51.605.120 B | 3.249.792 B | 1.981.729 / 28 | 1.439,9 MiB | 56,5 MiB |

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

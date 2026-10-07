# Seleção composta genérica: medição local

Execução em 2026-10-07, Go 1.26.0, wazero 1.12.0, Linux/amd64,
AMD Ryzen 5 5600GT, 12 CPUs disponíveis. Dados sintéticos e propriedades
fictícias; corpus e módulos do ambiente anterior não estão disponíveis.
Esta medição não reproduz os números históricos apresentados no pedido.
Sem metas de latência, p95/p99 ou comparação com outras linguagens.

A matriz do core/WASM e a saída local abaixo foram coletadas na implementação
inicial do PR #7. O ajuste de candidatos vazios não muda esses inputs, que têm
membros; esses tempos não foram novamente medidos. A coleta TCP foi atualizada
após restaurar `filtered` à filtragem sobre vizinhos (ver seção de transporte).

## Método

100.000 nodes, 100.000 edges formando um ciclo dirigido. Cada node tem `tag`
String (`V0` a `V6`, por ID módulo 7) e `rank` Int. IDs módulo 100 menores
que 6 recebem `L0` a `L5`; os demais recebem `OTHER`. `L0` também recebe
`L1`, exercitando sobreposição. Selecionar os seis labels e `tag == String(V0)`
produz 6.000 candidatos únicos e 857 nodes; o subgrafo induzido tem zero edges.
Cada amostra confere identidades/cardinalidade contra essa regra independente.

Mesmos CSV, ingestão heap com opções padrão, writer e mmap nos dois caminhos;
a única mudança de ingestão é `IndexProperties: []string{"tag"}`. Nenhuma
propriedade é privilegiada na engine. Três warmups por caso, 11 amostras,
execução sequencial, processo residente, snapshot já aberto/validado. Medianas
de amostras individuais; casos em ordem fixa, sem isolamento da máquina ou
inferência estatística de ganhos pequenos. `runtime.GC` antes da primeira
amostra de cada caso; `ReadMemStats` fica fora do intervalo cronometrado.

O baseline do **core** reproduz explicitamente a composição anterior:
para cada label, `NodesWithLabel`, busca **global** `NodesWithProperty`,
interseção e união. Preserva esse baseline mesmo após melhorar `Has`.
O caminho composto chama a API nova. Seleção exclui materialização de Subgraph,
serialização e transporte. Build do guest, compilação wazero, ingestão,
publicação e abertura são fases separadas no raw.

O comparativo **WASM** executa guests reais com `label.Has(...).Union(...)`
versus a seleção composta. Esse loop já usa o `Has` otimizado desta entrega,
portanto não representa o host antigo. Mede `ExecuteReport`, incluindo
instanciação WASI, execução e materialização via `Induced`, sem serialização.
Handles temporários são liberados em ambos os caminhos.

## Resultados

| Seleção do core | Sem índice (ms) | Com índice (ms) |
|---|---:|---:|
| Global por label (baseline) | 37.973 | 0.323 |
| Composta | 0.431 | 0.070 |

| Caso core | Índice | Bytes alocados / seleção | Alocações / seleção |
|---|---|---:|---:|
| core-global-per-label | não | 1,540,352 | 600,049 |
| core-global-per-label | sim | 340,352 | 49 |
| core-composed | não | 39,184 | 6,003 |
| core-composed | sim | 27,184 | 3 |

| Execução WASM + materialização | Sem índice (ms) | Com índice (ms) | Chamadas host |
|---|---:|---:|---:|
| Loop label.Has + Union | 5.830 | 5.335 | 40 |
| Composta | 5.280 | 4.978 | 4 |

| Caso WASM | Índice | Bytes alocados / execução | Alocações | Pico host (bytes) |
|---|---|---:|---:|---:|
| wasm-label-has | não | 9,581,056 | 65,435 | 50,016 |
| wasm-label-has | sim | 9,566,960 | 58,434 | 50,016 |
| wasm-composed | não | 9,318,064 | 63,759 | 50,016 |
| wasm-composed | sim | 9,306,080 | 57,758 | 50,016 |

| Fase de preparação (uma execução) | Sem índice (ms) | Com índice (ms) |
|---|---:|---:|
| build | 1106.107 | 1120.376 |
| write | 42.926 | 46.111 |
| open | 19.340 | 22.569 |

Build do guest: 143.733 ms; compilação wazero: 1141.707 ms (uma execução cada).

## Memória e interpretação

`AllocBytes`/`Allocs` são deltas de alocações do processo durante cada seleção
ou execução, não heap vivo ou RSS. O orçamento de bitmaps/filas é independente:
a seleção composta requer 25.008 bytes (candidatos e resultado) para este
corpus. O pico reportado do host no WASM inclui a materialização do subgrafo.
`MappedBytes` registra o tamanho do snapshot ativo mapeado, separado do heap.
`PeakRSSKiB` é o high-water cumulativo de `RUSAGE_SELF`, incluindo ingestão e
runtime; não pode ser atribuído isoladamente à query. Campos zero de fases sem
instrumentação de alocações/host não significam medição de custo zero.

O índice evita scans e a composição evita os bitmaps intermediários por label.
No WASM, a instanciação/execução pode dominar. A variação pequena do caso
indexado não sustenta uma promessa de aceleração de ponta a ponta.

Mapping ativo: 15,609,664 bytes sem índice e 16,009,984 com índice. Pico RSS cumulativo da execução inteira: 490,976 KiB.

## Saída local e transporte real

A saída local usa os mesmos 857 nodes, em memória, com buffer reutilizado:
materialização separada por `query.FromNodes`, graphjson e framing GGPB,
seguido de conversão GGPB → JSON para `io.Discard`. Não inclui filesystem ou
rede. Três warmups e 11 amostras por fase.

| Fase local | Sem índice (ms) | Com índice (ms) | Bytes |
|---|---:|---:|---:|
| materialization | 0.067 | 0.061 | — |
| serialization graphjson | 1.021 | 0.966 | 117313 |
| serialization ggpb-file | 0.485 | 0.424 | 40158 |
| conversion ggpb-file-to-json | 1.814 | 1.655 | 40158 |

A medição TCP usa uma **outra fixture**, pequena (5 nodes, 2 edges): `filtered`
versão 2 com filtragem sobre vizinhos via `RunWasm`, argumentos `S L tag X`,
resultado 2 nodes e 1 edge, 218 bytes GGPB em 3 batches. Cliente e servidor Go no mesmo processo,
TCP loopback real, sem TLS, concorrência 1. Três warmups e 11 amostras.
O caminho não usa GraphJSON; o cliente decodifica/valida batches GGPB.

| Fase TCP/GGPB | Sem índice (ms) | Com índice (ms) |
|---|---:|---:|
| EndToEndMS | 3.723 | 4.575 |
| ExecutionMaterializationMS | 3.486 | 4.379 |
| EncodeMS | 0.014 | 0.015 |
| SendMS | 0.004 | 0.006 |
| ClientDecodeMS | 0.013 | 0.015 |

`ExecutionMaterializationMS`, `EncodeMS` e `SendMS` usam a instrumentação
existente do servidor. Send mede a chamada ao gRPC, incluindo cópia/envio
local, sem representar isoladamente o tempo na rede. End-to-end inclui
transporte, execução, recepção e decode do cliente; fases podem sobrepor e
não devem ser somadas ou subtraídas para inventar latência de rede. Não há
conversão para objetos de outro cliente nem medição de processo externo aqui.
Os números dessa fixture não projetam o custo de saída dos 857 nodes ou do
corpus anterior.

## Reprodução

Dependências disponíveis no cache; execute as medições sem race/profiler e
sem outros gates concorrentes. Os arquivos destino devem ter diretório existente.

```sh
GOPROXY=off GOTOOLCHAIN=local \
  GOPHERGRAPH_SELECTION_MEASURE=/tmp/selection.jsonl \
  go test -count=1 ./wasmquery -run '^TestSelectionMeasurement$'
# O teste remoto precisa de permissão para escutar TCP loopback.
# Este arquivo é append: remova apenas seu próprio raw antes de outra execução.
GOPROXY=off GOTOOLCHAIN=local \
  GOPHERGRAPH_SELECTION_TRANSPORT_MEASURE=/tmp/selection-transport.jsonl \
  go test -count=1 ./remote -run '^TestFilteredSelectionStreaming$'
```

[Raw seleção/saída local](benchmarks/selection_samples.jsonl) e
[raw transporte](benchmarks/selection_transport.jsonl). Redução das medianas
pela stdlib Python, sem subsistema de profiling novo:

```python
import collections, json, statistics
rows = [json.loads(line) for line in open("docs/benchmarks/selection_samples.jsonl")]
groups = collections.defaultdict(list)
for row in rows:
    groups[row["Phase"], row["Mode"], row["Indexed"]].append(row)
for key, samples in groups.items():
    print(key, len(samples), statistics.median(x["Millis"] for x in samples))
```

`EdgeSet.Has`, novas operações de expansão/alcance, projeção de resposta,
RPCs específicos, planner, formato de snapshot e modelagem transacional não
fazem parte desta entrega. Nenhuma otimização adicional de saída foi aplicada.

## Regressões da revisão

`TestEmptySelectionSkipsPropertyWork` usa 100.000 postings coincidentes e um
conjunto vazio. O limite observado cobre somente verificações no bitmap;
a implementação anterior ultrapassou esse limite (100–102 checks). Abrange
Has, labels nil/vazios/desconhecidos, index/scan, ownership e cancelamento.

`TestFilteredPropertyWorkStaysWithNeighbors` mantém 5.000 nodes/propriedades
e um único vizinho, variando a frequência de `L` de 1 para 5.000. Strings de
propriedades em mmap tornam os decodes observáveis em alocações. Três execuções
por caso com `testing.AllocsPerRun`, após warmup próprio, sem assert de tempo.
A versão que seleciona globalmente antes de intersectar é rejeitada; a versão
restrita aos vizinhos mantém o trabalho. Os valores observados constam em
PROGRESS. Essas contagens não representam RSS nem o orçamento do host.

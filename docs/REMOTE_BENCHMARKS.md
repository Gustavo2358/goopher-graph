# Qualificação do servidor residente

Campanha explícita sobre dados sintéticos; nenhuma promessa de SLA. Hardware:
Ryzen 5 5600GT, Linux/amd64, Go 1.26.0, 12 CPUs disponíveis, grpc-go 1.84.0,
wazero 1.12.0, Protobuf Go 1.36.12; clientes oficiais Go/Python/Java. TCP loopback
real, processos separados na campanha de clientes. Builds/preparação separados
antes das amostras. HTTP métricas coletadas a cada 100 ms (overhead incluído),
sem profiler/race no benchmark. RSS/heap/goroutines peak são máximos amostrados,
não medição contínua exata. A máquina não é host dedicado isolado.

## Comparação do codec

Corpus sintético existente: 400.000 nodes/2.000.000 edges, snapshot ~197 MiB,
territory 360.000 nodes/1.800.000 edges, ~88,22 MB GGPB por resultado. Warm:
memlock do ambiente é 8 MiB, então esse corpus não pode cumprir locked aqui.
Três repetições de dez operações por caminho; valores abaixo são medianas das
três médias, não p99. Sink exclui query e framing; TCP inclui query, wrapper,
clone onde necessário e transporte, sem decode GGPB do cliente. Generated
marshal vive somente no benchmark, sem segunda API de produção.

| Caminho | ms/op | B/op aproximadamente |
|---|---:|---:|
| EmitEncoded → sink | 677,59 | 41.778.244 |
| Emit → generated marshal → sink | 1401,30 | 132.781.552 |
| EmitEncoded → clone → EncodedBatch → TCP | 1016,12 | 238.157.368 |
| Emit → generated marshal → EncodedBatch → TCP | 1672,33 | 238.257.820 |

Fast path reduziu tempo TCP em cerca de **39,2%**, mesmo com ownership e wrapper
padrão; redução no sink foi ~51,6%. Allocated bytes/op TCP ficaram próximos:
clone da saída emprestada custa o que o caminho generated já aloca em marshal.
Não é zero-copy e B/op não é heap live/RSS. [Raw](benchmarks/remote_codec.txt).
Os números anteriores 25/186/330/575 ms pertencem a outra escala/campanha;
não devem ser tratados como baseline equivalente desta execução.

Reprodução, após gerar o corpus com os tools existentes:

```sh
GOPHERGRAPH_BENCH_SNAPSHOT=/caminho/absoluto/graph.snapshot \
  go test ./remote -run '^$' -bench BenchmarkCorpusTransport -benchtime=10x -count=3
```

`BenchmarkEncodedTransport` oferece o mesmo comparativo com fixture compacta
locked e valores 64/2048 bytes. Nenhuma dependency download durante o teste.

## Matriz de clientes

Fixtures compactas de ciclo conectadas, N=E: small 100, medium/large 4000.
Property value é internado uma vez no snapshot e repetido no resultado:
64 bytes small/medium, 2048 large. Assim o payload grande cabe em snapshot
locked menor que 8 MiB. Mantém seleção/set sizes constantes ao variar bytes.
Native Territory e WASM shared-targets(same,same), 100 requests por combinação,
concorrência 1/2/4/8/16, Go e Python, locked/warm/lazy. Global/WASM capacity 16
na campanha, orçamento de query 4 GiB; não são defaults/recomendação universal.
Default continua bootstrap derivado de CPU e envelope de memória.

[Raw JSONL](benchmarks/remote_samples.jsonl): percentis, throughput, bytes,
batches, tempos de decode, status, CPU/RSS/heap/goroutines e deltas de métricas
por fase/alocações/GC. Percentil usa índice floor((n-1)*p); p99 só com >=100
sucessos. Python mede validação estrutural/typed e decode no consumidor, sob
GIL; paralelizar threads não remove esse custo. Go valida incrementalmente.
180 combinações/18.000 RPCs terminaram OK, sem rejects/cancels nesta carga.
60 combinações adicionais/6.000 RPCs mediram resultados vazios nativos/WASM
([raw](benchmarks/remote_empty.jsonl)). Exemplos do payload large, locked:

| Cliente | Query | Concorrência | p50 ms | p95 ms | p99 ms | queries/s |
|---|---|---:|---:|---:|---:|---:|
| go | territory | 1 | 25.07 | 27.00 | 29.20 | 39.57 |
| go | territory | 4 | 63.67 | 69.74 | 71.03 | 62.21 |
| go | territory | 16 | 295.25 | 307.10 | 309.73 | 54.11 |
| go | shared-targets | 1 | 31.24 | 33.65 | 34.90 | 31.94 |
| go | shared-targets | 4 | 70.04 | 76.09 | 77.75 | 56.89 |
| go | shared-targets | 16 | 315.36 | 324.28 | 327.05 | 51.20 |
| python | territory | 1 | 43.51 | 45.70 | 46.71 | 22.93 |
| python | territory | 4 | 193.21 | 216.26 | 228.70 | 20.54 |
| python | territory | 16 | 868.09 | 927.57 | 975.34 | 18.41 |
| python | shared-targets | 1 | 48.59 | 50.92 | 52.14 | 20.54 |
| python | shared-targets | 4 | 195.41 | 214.39 | 223.25 | 20.32 |
| python | shared-targets | 16 | 865.70 | 955.97 | 996.37 | 18.44 |

Concorrência 4 aumentou throughput Go; 16 elevou caudas e memória sem ganhar
throughput nesse payload. Python ficou limitado pelo decode/validação sob GIL:
concorrência não melhorou sua vazão. Não extrapolar isso para clientes que
consomem raw bytes ou workloads/CPUs diferentes. Locked/warm/lazy ficaram
próximos porque snapshots pequenos já estavam aquecidos após validação/hash,
sem pressão de reclaim. Não é prova de irrelevância de locking.

Máximo amostrado na matriz: RSS 325,73 MiB, goroutines 36; registrar alocações
acumuladas separadamente de heap/RSS. Servidor incluiu guests/cache/wazero.
Query capacity default continua uma estimativa segura, não o valor 16 da carga.


```sh
CGO_ENABLED=0 go build -o bin/gophergraph-remotefixture ./tools/remotefixture
CGO_ENABLED=0 go build -o bin/gophergraph-remotemeasure ./tools/remotemeasure
bin/gophergraph-remotefixture --nodes=100 --scalar=64 --output=.measure/remote-small.snapshot
bin/gophergraph-remotefixture --nodes=4000 --scalar=64 --output=.measure/remote-medium.snapshot
bin/gophergraph-remotefixture --nodes=4000 --scalar=2048 --output=.measure/remote-large.snapshot
python tools/remote_campaign.py
```

O Python desses comandos deve ser o venv com as requirements do cliente.
Para a campanha vazia e probe RSS, preparar também:

```sh
go run ./tools/benchdata --nodes=100 --output=.measure/remote-empty-input
bin/gophergraph build --nodes=.measure/remote-empty-input/nodes \
  --edges=.measure/remote-empty-input/edges --output=.measure/remote-empty.snapshot
bin/gophergraph-remotefixture --nodes=4000 --scalar=32768 --output=.measure/remote-wide.snapshot
python tools/remote_empty.py
python tools/remote_resilience.py
```

## Backpressure e shutdown

TCP real, mesmo mmap locked de 4k nodes/4k edges. Property payload lógico
16.384.000 → 262.144.000 bytes: heap live pausado cresceu 3.666.536 → 3.512.440
bytes, com threshold constante de 24 MiB. Token permanece ativo e nova query é
RESOURCE_EXHAUSTED. Cancel e deadline em Send bloqueado liberam admission;
shutdown com graça 30 ms cancela transporte e join ocorre antes de Graph.Close.
Oito native/WASM simultâneas usam o mesmo mapping; compile permanece 3.

Probe com Python parado e servidor em processo separado passou em locked:
Variação de RSS foi -18.76 MiB para payload lógico 16,384 MB e
0.45 MiB para 262,144 MB; limite constante 32 MiB,
overload RESOURCE_EXHAUSTED e cancel cleanup confirmado.
[Raw RSS](benchmarks/remote_resilience.json); `python tools/remote_resilience.py`.
A variação negativa reflete GC/scavenging após o startup; não significa que
o envio não alocou memória. O critério limita crescimento com consumidor parado.

O teste de upstream reproduziu `grpc-go=1.84.0 concurrent-stop=blocked` em
subprocesso protegido. Nosso lifecycle não chama Stop após iniciar GracefulStop.
Health.Watch ativo também não impede encerramento. Race instrumenta Go, não
prova comportamento de kernel/JIT ou todas as interleavings possíveis.

## Gates e limites de qualificação

Normal/race completos, vet, gofmt/diff check, build sem cgo; fuzz CSV/snapshot,
GGPB batches/reader/wire e módulo WASM; interoperabilidade Go/Python/Java;
startup fail-closed com memlock zero e erros de prefault, overlimit requests,
conexões, contexto/cancel/cleanup e formato. Comandos/resultados exatos ficam
somente em PROGRESS.

Loopback comprova transporte real, não substitui medição WAN/LAN/TLS no destino.
TLS possui teste de handshake e query streaming, não um SLA de throughput.
Lazy não foi precedido por drop de caches do host; validação/hash já leem bytes.
Não há comparação falsa de cold page faults com caches quentes. Repetir com
snapshots reais, limites/cgroup e consumidores do deployment antes de escolher
capacidade de produção. Memlock/cgroup OOM são condições do ambiente, sem
autoelevação de privilégios pelo servidor. Não foram alterados limites do host
ou criados manifests Kubernetes. Hot-swap/HA continuam fora de escopo.

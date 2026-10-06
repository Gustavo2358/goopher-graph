# Campanha JSON vs GGPB — 2026-10-06

Medições locais reais, cinco processos frescos por combinação; valores abaixo
são medianas. Linux/amd64, Go 1.26.0, AMD Ryzen 5 5600GT (12 CPUs lógicas).
A fixture existente `internal/benchfixture` gera dois componentes desconectados,
chains, diamantes, ciclos, hubs, loops e propriedades String/Int. Snapshots têm
100/500, 10.000/50.000 e 100.000/500.000 nodes/edges; as queries abaixo retornam
90% do componente principal. Between entre componentes produz o caso vazio.
Não houve seleção de queries favoráveis. Os três comandos nativos foram medidos. WASM também aceita GGPB e tem paridade
E2E completa/parcial; sua execução não foi incluída na campanha de performance.

Fixture generation, ingestão, build do binário e abertura/validação do mmap ficam
fora de query/encoding. Cada processo abre exatamente o mesmo snapshot e executa
a query antes de escrever em arquivo local. Producer total = query + encoding;
encoding inclui escrita e Close, sem fsync. Não representa latência de rede,
gRPC ou durabilidade de disco. Ordem de formatos alternada por repetição; page
cache aquecido, GC padrão, sem pools ou unsafe no código próprio, sem tuning de GOGC. Não houve testes ou
benchmarks concorrentes na coleta final. CPU é user+system via getrusage, somente
query+encoding (ou decode). RSS é VmHWM de /proc do executable, incluindo open,
mapping, query e serialization; não inclui ingestão nem compilação. getrusage RSS
pode herdar HWM do processo Python anterior ao exec, por isso não foi usado como
fonte principal de RSS. Não interpretar RSS como heap exclusivo do encoder.

[300 amostras brutas](benchmarks/ggpb_samples.jsonl) incluem tempos, CPU, allocs,
heap e bytes. Throughput usa bytes do respectivo formato por segundo; payloads
menores podem ter MB/s menor mesmo quando levam menos tempo.

## Produtor: tamanho, encoding e RSS

Redução = 1 − GGPB bytes / JSON bytes. Speedup = JSON encode / GGPB encode;
abaixo de 1 significa GGPB mais lento. Tempos em ms; RSS em MiB.

| Caso | Nodes | Edges | JSON bytes | GGPB bytes | Redução | JSON encode | GGPB encode | Speedup | Peak RSS JSON | Peak RSS GGPB |
|---|---|---|---|---|---|---|---|---|---|---|
| 100 territory | 90 | 450 | 72103 | 29850 | 58.60% | 0.702 | 0.958 | 0.73x | 5.17 | 5.72 |
| 100 anti-territory | 90 | 450 | 72108 | 29855 | 58.60% | 0.635 | 0.966 | 0.66x | 5.14 | 5.73 |
| 100 between | 90 | 450 | 72119 | 29860 | 58.60% | 0.736 | 0.988 | 0.74x | 5.18 | 5.75 |
| 100 between vazio | 0 | 0 | 178 | 67 | 62.36% | 0.045 | 0.173 | 0.26x | 5.01 | 5.28 |
| 10000 territory | 9000 | 45000 | 7212557 | 2984088 | 58.63% | 57.623 | 53.164 | 1.08x | 14.48 | 15.33 |
| 10000 anti-territory | 9000 | 45000 | 7212562 | 2984093 | 58.63% | 56.534 | 53.197 | 1.06x | 14.43 | 15.17 |
| 10000 between | 9000 | 45000 | 7212573 | 2984098 | 58.63% | 56.528 | 52.434 | 1.08x | 14.62 | 14.99 |
| 100000 territory | 90000 | 450000 | 72214059 | 29914323 | 58.58% | 556.872 | 517.118 | 1.08x | 57.30 | 57.94 |
| 100000 anti-territory | 90000 | 450000 | 72214064 | 29914328 | 58.58% | 553.868 | 516.500 | 1.07x | 57.25 | 57.80 |
| 100000 between | 90000 | 450000 | 72214075 | 29914333 | 58.58% | 557.652 | 516.440 | 1.08x | 57.52 | 58.04 |

## Fases e CPU do produtor

Cada célula J/G apresenta JSON e GGPB. Open inclui validação do snapshot;
Total omite open e inicialização do processo. CPU pode superar wall time com GC
em threads auxiliares. Não se deve somar medianas para recriar a mediana do total.

| Caso | Open ms J/G | Query ms J/G | Serialization ms J/G | Total ms J/G | CPU ms J/G |
|---|---|---|---|---|---|
| 100 territory | 0.088 / 0.084 | 0.040 / 0.042 | 0.702 / 0.958 | 0.743 / 0.999 | 0.830 / 1.102 |
| 100 anti-territory | 0.086 / 0.094 | 0.036 / 0.043 | 0.635 / 0.966 | 0.671 / 1.012 | 0.751 / 1.104 |
| 100 between | 0.085 / 0.087 | 0.059 / 0.060 | 0.736 / 0.988 | 0.804 / 1.048 | 0.892 / 1.142 |
| 100 between vazio | 0.084 / 0.085 | 0.029 / 0.029 | 0.045 / 0.173 | 0.074 / 0.207 | 0.126 / 0.255 |
| 10000 territory | 5.576 / 5.616 | 2.790 / 2.695 | 57.623 / 53.164 | 60.390 / 56.052 | 63.386 / 63.654 |
| 10000 anti-territory | 5.560 / 5.645 | 2.862 / 2.831 | 56.534 / 53.197 | 59.462 / 55.910 | 62.367 / 63.572 |
| 10000 between | 5.794 / 5.651 | 4.196 / 4.183 | 56.528 / 52.434 | 60.782 / 56.622 | 63.753 / 64.583 |
| 100000 territory | 57.008 / 56.280 | 26.178 / 26.160 | 556.872 / 517.118 | 583.544 / 543.243 | 611.249 / 614.747 |
| 100000 anti-territory | 56.672 / 56.338 | 27.209 / 27.201 | 553.868 / 516.500 | 581.778 / 543.701 | 607.795 / 616.395 |
| 100000 between | 56.574 / 55.989 | 40.890 / 41.006 | 557.652 / 516.440 | 599.287 / 557.105 | 628.416 / 631.273 |

## Consumidor incremental

Outro processo lê cada arquivo. JSON usa encoding/json e uma entidade por vez,
com parsing de todo valor conforme tipo declarado. GGPB usa Reader.Next, valida
framing/checksum/continuidade/contagens e todo valor tipado, mantendo um frame.
Ambos percorrem todas as entidades e verificam contagens, sem reter o resultado.
Não há exportação JSON no custo de decode GGPB. Os validadores não são idênticos,
mas ambos realizam parsing semântico de todos os valores.

| Caso | JSON decode ms | GGPB decode ms | Speedup | MB/s J/G | RSS MiB J/G | CPU ms J/G |
|---|---|---|---|---|---|---|
| 100 territory | 1.451 | 0.887 | 1.64x | 49.7 / 33.7 | 5.25 / 5.59 | 1.537 / 0.952 |
| 100 anti-territory | 1.329 | 0.861 | 1.54x | 54.3 / 34.7 | 5.16 / 5.63 | 1.413 / 0.919 |
| 100 between | 1.447 | 0.894 | 1.62x | 49.8 / 33.4 | 5.25 / 5.62 | 1.543 / 0.958 |
| 100 between vazio | 0.049 | 0.179 | 0.28x | 3.6 / 0.4 | 4.95 / 5.23 | 0.069 / 0.212 |
| 10000 territory | 116.003 | 44.289 | 2.62x | 62.2 / 67.4 | 10.02 / 10.59 | 118.392 / 51.040 |
| 10000 anti-territory | 115.465 | 44.531 | 2.59x | 62.5 / 67.0 | 9.97 / 10.84 | 118.319 / 50.633 |
| 10000 between | 116.587 | 45.147 | 2.58x | 61.9 / 66.1 | 9.86 / 10.59 | 119.342 / 51.004 |
| 100000 territory | 1141.110 | 423.766 | 2.69x | 63.3 / 70.6 | 10.30 / 10.97 | 1163.985 / 482.963 |
| 100000 anti-territory | 1135.143 | 425.578 | 2.67x | 63.6 / 70.3 | 10.00 / 11.09 | 1158.012 / 484.167 |
| 100000 between | 1136.589 | 425.607 | 2.67x | 63.5 / 70.3 | 10.08 / 11.21 | 1160.959 / 481.287 |

## Throughput e alocações do produtor

Alocações abrangem query+encode e são cumulativas, não heap vivo.

| Caso | Encoding MB/s J/G | TotalAlloc MiB J/G | Allocs J/G |
|---|---|---|---|
| 100 territory | 102.7 / 31.2 | 0.13 / 0.51 | 9378 / 10414 |
| 100 anti-territory | 113.6 / 30.9 | 0.13 / 0.51 | 9377 / 10414 |
| 100 between | 98.0 / 30.2 | 0.13 / 0.51 | 9388 / 10424 |
| 100 between vazio | 3.9 / 0.4 | 0.01 / 0.07 | 114 / 529 |
| 10000 territory | 125.2 / 56.1 | 11.98 / 35.32 | 936041 / 931934 |
| 10000 anti-territory | 127.6 / 56.1 | 11.98 / 35.32 | 936043 / 931930 |
| 10000 between | 127.6 / 56.9 | 12.12 / 35.46 | 936069 / 931947 |
| 100000 territory | 129.7 / 57.8 | 120.18 / 352.13 | 9360206 / 9309138 |
| 100000 anti-territory | 130.4 / 57.9 | 120.18 / 352.13 | 9360235 / 9309135 |
| 100000 between | 129.5 / 57.9 | 122.08 / 354.03 | 9360250 / 9309164 |

## Dicionário vs strings inline

Mesma versão e esquema, com Options.InlineSymbols=true como baseline.
O dicionário cobre só labels/keys e reinicia por batch. Todos os casos medidos
são apresentados; não foi internado o texto repetido das propriedades.

| Caso | Inline bytes | Dictionary bytes | Redução vs inline | Encode ms inline/dict | Decode ms inline/dict |
|---|---|---|---|---|---|
| 100 territory | 34096 | 29850 | 12.45% | 1.000 / 0.958 | 0.892 / 0.887 |
| 100 anti-territory | 34101 | 29855 | 12.45% | 0.981 / 0.966 | 0.870 / 0.861 |
| 100 between | 34106 | 29860 | 12.45% | 0.942 / 0.988 | 0.875 / 0.894 |
| 100 between vazio | 67 | 67 | 0.00% | 0.178 / 0.173 | 0.140 / 0.179 |
| 10000 territory | 3412371 | 2984088 | 12.55% | 51.954 / 53.164 | 47.751 / 44.289 |
| 10000 anti-territory | 3412376 | 2984093 | 12.55% | 51.967 / 53.197 | 46.666 / 44.531 |
| 10000 between | 3412381 | 2984098 | 12.55% | 51.791 / 52.434 | 47.481 / 45.147 |
| 100000 territory | 34197379 | 29914323 | 12.52% | 504.458 / 517.118 | 444.055 / 423.766 |
| 100000 anti-territory | 34197384 | 29914328 | 12.52% | 505.074 / 516.500 | 442.641 / 425.578 |
| 100000 between | 34197389 | 29914333 | 12.52% | 503.542 / 516.440 | 444.929 / 425.607 |

## O que a hipótese confirmou e o que não confirmou

- Query manteve o algoritmo e tempos próximos: territory grande 26,178 / 26,160
  ms (JSON/GGPB). A diferença é variação de medição; serializar não executa traversal.
- Payload caiu 58,58–58,63% nos resultados não vazios. No caso grande territory,
  72.214.059 → 29.914.323 bytes. O vazio caiu 178 → 67 bytes.
- Encoding médio/grande ficou 1,06–1,08x mais rápido, um ganho modesto. Territory
  grande: 556,872 → 517,118 ms; total query+encode: 583,544 → 543,243 ms.
- **CPU do produtor não caiu**: territory grande 611,249 → 614,747 ms (+0,6%);
  anti-territory +1,4%, between +0,5%. A hipótese de menor CPU não foi confirmada.
  Allocations de objetos Protobuf, cópias de strings pelo backend mmap, marshaling,
  CRC32 e GC continuam tendo custo; não atribuir a diferença a uma causa única
  sem profiling adicional. TotalAlloc grande passou de ~120 para ~352 MiB.
- **GGPB ficou mais lento nos casos pequenos/vazio**, tanto no produtor como
  no consumidor vazio. Inicialização de reflection/descritores e framing têm
  custo fixo em processos novos. Engine residente pode ter comportamento diferente,
  mas não foi medida aqui.
- **RSS não melhorou**: territory grande 57,30 → 57,94 MiB; consumidor
  10,30 → 10,97 MiB. Resultado limitado em memória não significa menos memória
  que JSON. Heap auxiliar vivo amostrado é uma evidência separada abaixo.
- Consumidor médio/grande ficou 2,58–2,69x mais rápido e usou menos CPU:
  territory grande decode 1.141,110 → 423,766 ms, CPU 1.163,985 → 482,963 ms.
  Assim, a campanha não mostra simplesmente o custo de texto transferido para
  outro processo; consumir diretamente o binário teve benefício mensurável.
- Dictionary local reduziu mais 12,45–12,55% contra o GGPB inline, com ~2,5%
  a mais de encode no territory grande e decode ligeiramente melhor. Foi mantido
  pelo benefício de bytes, com baseline disponível; não se afirma que sempre
  melhora CPU. Labels/keys de alta cardinalidade usam fallback inline.

O encoder inicial usava cálculos recursivos de proto.Size por property, part e
batch além do marshal. A primeira medição grande ficou em aproximadamente 840 ms
(query+encode), pior que JSON (~585 ms). Substituir estimativas intermediárias por
limites conservadores e reutilizar o size cache validado no marshal reduziu esse
custo. O limite final ainda é verificado por proto.Size antes da emissão. Não foi
adotado um wire codec próprio ou uma arena genérica para perseguir a hipótese.

O corpus é determinístico/sintético e tem topologia/propriedades reais para
exercitar o pipeline completo; não é dado corporativo nem todas as distribuições
possíveis. Cinco repetições não estabelecem SLA nem significância estatística.
Não se mediu gRPC, Lambda Python, cold cache ou efeitos de longa residência.

## Memória, paridade e determinismo fora dos tempos de benchmark

`TestBoundedMemory` descarta bytes, amostra heap vivo com GC a cada ~16 MiB e
usa o mesmo limite de 8 MiB em 1.000 e 100.000 nodes/edges. A saída maior tem
214.422.538 bytes; não é retida pelo teste. Há também limite de 256 KiB de
TotalAlloc antes da primeira escrita que falha. Essas amostras não capturam todos
os picos entre pontos; os limites de batch/frame e iteradores são a garantia de
retenção independente do tamanho total. LabelIterator não aloca; uma entidade
com 30 mil propriedades e labels repartidos em partes passam pelo round-trip.

Os seis snapshots golden existentes e casos de extremos numéricos/UTF-8 geram
JSON byte a byte idêntico após GGPB → JSON. A CLI foi comparada nos três comandos,
completo/parcial, filtros, stdout/arquivo e ID vazio. No caso grande, a conversão
em processo separado também foi comparada com cmp ao JSON direto de 72 MB.
Três emissões do caso grande produziram o mesmo SHA-256:
`1b57c17ea20adc3cd420d8ff9494a92d32a0759ffc883abf96f307e4cc51bd15`.
O script de campanha agora verifica hashes em todas as cinco repetições por caso.
As amostras brutas registram hashes dos producers. Heap auxiliar vivo amostrado
no teste final: 3.960 bytes (1.000 entidades por catálogo) e 365.752 bytes
(100.000 entidades por catálogo), usando o mesmo limite fixo. Consumidor Python oficial
Protobuf validou o arquivo grande e fixtures typed/numeric/presence, incluindo
os dez tipos, propriedades multivaloradas e parcialidade.

## Reprodução

```sh
# Dependências Go já disponibilizadas por go mod download; não há download no teste.
mkdir -p bin
go build -o bin/benchdata ./tools/benchdata
CGO_ENABLED=0 go build -o bin/gophergraph ./cmd/gophergraph
go build -o bin/resultmeasure ./tools/resultmeasure
python3 tools/ggpb_campaign.py --bin "$PWD/bin" --work /tmp/ggpb-campaign --repeats 5

# Uma medição de cada processo, sem compilação no intervalo:
bin/resultmeasure --snapshot /tmp/ggpb-campaign/100000/graph.snapshot \
  --query territory --format ggpb --output /tmp/result.ggpb
bin/resultmeasure --mode decode --format ggpb --input /tmp/result.ggpb

# Benchmarks Go sobre a mesma fixture, além da campanha de processos:
go test -run '^$' -bench . -benchmem -benchtime=1x ./...
```

O driver de medição Linux usa getrusage e /proc; não é parte do CLI de produto.
Os bytes emitidos são idênticos aos da CLI. Consumer não converte para JSON durante
medição; `gophergraph decode` é o caminho de conversão para uso e testes de paridade.
Nesta sessão, GOCACHE foi colocado em /tmp (cache padrão não gravável) e TMPDIR
em `.measure/tmp` (o ambiente tinha `/tmp/.git` inválido). Nenhum teste foi
relaxado e nenhum arquivo de fixture/golden original foi modificado.

## Benchmarks Go com io.Discard (uma iteração)

O gate completo `-bench . -benchmem -benchtime=1x ./...` também passou. Nesse
sink, o caso grande GGPB foi **1,9% mais lento**; não ocultar esse resultado nem
interpretar uma iteração como distribuição estatística. Esse caminho inclui
encoding e framing/checksum, sem syscalls de escrita em arquivo. O ganho de wall
time com arquivo não estabelece menor custo de CPU do encoder.

| Snapshot N/E | JSON encode ms | GGPB encode ms | JSON B/op | GGPB B/op |
|---|---|---|---|---|
| 1.000 / 5.000 | 5,263 | 6,188 | 1.249.928 | 3.833.232 |
| 10.000 / 50.000 | 50,586 | 48,109 | 12.396.512 | 36.727.432 |
| 100.000 / 500.000 | 500,971 | 510,652 | 123.920.288 | 366.996.792 |

## Campanha de otimização: checkpoint 0

Baseline capturado antes de alterar o encoder, no HEAD a6e3d32. CPU profile cobre seis encodings com io.Discard após um warmup, snapshot/query abertos uma vez; memprofile amostrado inclui open/query e sete encodings (warmup incluído). Perfis de heap foram escritos após GC: não equivalem a peak heap. [Hotspots](benchmarks/ggpb_profiles/baseline-ggpb-cpu.txt), [alloc_space](benchmarks/ggpb_profiles/baseline-ggpb-alloc_space.txt), [alloc_objects](benchmarks/ggpb_profiles/baseline-ggpb-alloc_objects.txt). Tabelas de JSON/inline estão no mesmo diretório. Dados brutos dos quatro casos em [c0](benchmarks/ggpb_c0.jsonl).

No GGPB, mallocgc soma 23,9% CPU cumulativa, marshalAppendPointer 21,3%, sizePointer 14,4%. Emit/property/closures concentram a alocação; Data.String representa 5,85% alloc_space e 15,74% alloc_objects. CRC/syscalls não dominam esse perfil com Discard. Porcentagens cumulativas têm sobreposição e não devem ser somadas.

| Caso | Formato | Encode ms | CPU query+encode ms | TotalAlloc MiB | Allocs | Bytes | Consumer ms |
|---|---|---|---|---|---|---|---|
| 100 | json | 0.722 | 0.820 | 0.13 | 9377 | 72103 | 1.384 |
| 100 | ggpb | 0.891 | 1.021 | 0.51 | 10414 | 29850 | 0.957 |
| 100 | ggpb-inline | 0.885 | 1.019 | 0.49 | 10398 | 34096 | 0.864 |
| 10000 | json | 59.423 | 64.975 | 11.98 | 936037 | 7212557 | 122.044 |
| 10000 | ggpb | 59.022 | 70.807 | 35.32 | 931932 | 2984088 | 49.045 |
| 10000 | ggpb-inline | 55.947 | 66.918 | 35.26 | 931079 | 3412371 | 49.366 |
| 100000 | json | 556.494 | 610.529 | 120.18 | 9360212 | 72214059 | 1143.122 |
| 100000 | ggpb | 510.982 | 607.138 | 352.13 | 9309134 | 29914323 | 422.519 |
| 100000 | ggpb-inline | 605.020 | 702.232 | 351.50 | 9300716 | 34197379 | 596.339 |
| empty | json | 0.058 | 0.129 | 0.01 | 114 | 178 | 0.052 |
| empty | ggpb | 0.185 | 0.270 | 0.07 | 529 | 67 | 0.165 |
| empty | ggpb-inline | 0.168 | 0.240 | 0.07 | 529 | 67 | 0.182 |

### A — StringID-aware symbols (KEEP)

Dicionário e cache bounded indexados por StringID somente dentro do adapter. Resolve labels/keys uma vez por batch, com fallback fora do limite. Os refs transitórios são traduzidos antes do callback e nunca saem no wire. Nenhuma extensão do core. Grande c0→c1: 510,982→484,577 ms (−5,2%), CPU 607,138→585,396 ms; allocations 9.309.134→8.506.529 (−8,6%). Payload byte a byte inalterado. Mantido por ganho de tempo e eliminação de resoluções repetidas; cache extra continua limitado a 1.024 entradas/64 KiB.

[Dados brutos](benchmarks/ggpb_c1.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.857 | 0.985 | 0.51 | 9618 | 29850 | 0.847 | 0.580 |
| 10000 | 53.364 | 62.906 | 34.86 | 851673 | 2984088 | 46.141 | 55.408 |
| 100000 | 484.577 | 585.396 | 347.59 | 8506529 | 29914323 | 417.655 | 491.571 |
| empty | 0.175 | 0.251 | 0.07 | 528 | 67 | 0.166 | 0.017 |

### B — referências locais de endpoints (KEEP)

Records.endpoint_ids contém uma tabela lógica local de até 512 IDs externos/64 KiB; EdgePart.source_ref/target_ref são ordinais 1-based locais, nunca NodeID. Fallback inline para limites. Cache privado por NodeID elimina resoluções repetidas; refs transitórios não saem do adapter. Tabela global/ordinal de todos os nodes foi descartada por exigir retenção proporcional ao resultado; rank sem índice implicaria scan repetido dos sets. Grande: payload 29.914.323→22.070.295 bytes (−26,2% adicionais), allocations 8.506.529→7.746.858. Encode 484,577→498,489 ms (+2,9%) e CPU 585,396→596,853 (+2,0%); mantido pelo benefício substancial de bytes, explicitando o custo. Schema v1 ainda não estabilizado no PR; string source/target continuam como fallback legítimo, não protocolo experimental.

[Dados brutos](benchmarks/ggpb_c2.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 1.066 | 1.215 | 0.49 | 8905 | 22026 | 0.884 | 0.632 |
| 10000 | 54.391 | 66.205 | 34.80 | 775740 | 2199660 | 41.642 | 54.687 |
| 100000 | 498.489 | 596.853 | 346.87 | 7746858 | 22070295 | 377.994 | 499.497 |
| empty | 0.153 | 0.214 | 0.07 | 530 | 67 | 0.196 | 0.020 |

### C1 — reuso bounded de objetos/slices (KEEP)

Slots locais concretos reutilizam Part/NodePart/EdgePart, Symbol/Property e wrappers tipados após callback síncrono. Sem pool global, unsafe ou allocator genérico. Partes grandes (mais de 16 properties/labels de capacidade) são liberadas; cache tem no máximo 256 slots. Grande c2→C1: 498,489→349,586 ms (−29,9%); CPU 596,853→387,062 ms; TotalAlloc 346,87→34,06 MiB (−90,2%); allocations 7.746.858→819.162. Mesmos bytes. O buffer MarshalAppend já era reutilizado no baseline; C2 abaixo avalia maps/Records também.

[Dados brutos](benchmarks/ggpb_c3-reuse.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.901 | 1.042 | 0.41 | 3636 | 22026 | 0.767 | 0.560 |
| 10000 | 36.834 | 40.966 | 3.79 | 85044 | 2199660 | 41.943 | 36.594 |
| 100000 | 349.586 | 387.062 | 34.06 | 819162 | 22070295 | 374.213 | 357.199 |
| empty | 0.173 | 0.242 | 0.07 | 531 | 67 | 0.143 | 0.018 |

### C2 — maps, Records e buffers (KEEP)

Reuso por query dos maps locais (clear após callback), Records e slices de dictionary/parts/endpoints. MarshalAppend já reutilizava o buffer, portanto não se atribui novo ganho a esse baseline existente. Grande: 349,586→331,488 ms (−5,2%), CPU 387,062→363,687; TotalAlloc 34,06→12,34 MiB (−63,8%), Allocs 819.162→739.358. Payload idêntico; teste bounded-memory passou.

[Dados brutos](benchmarks/ggpb_c3-buffers.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.905 | 1.029 | 0.40 | 3573 | 22026 | 0.911 | 0.492 |
| 10000 | 33.905 | 36.980 | 1.62 | 77092 | 2199660 | 39.700 | 32.949 |
| 100000 | 331.488 | 363.687 | 12.34 | 739358 | 22070295 | 375.942 | 328.838 |
| empty | 0.153 | 0.220 | 0.07 | 530 | 67 | 0.163 | 0.016 |

### D1 — eliminar Size externo com marshal gerado (REJECT)

Protótipo usou limite conservador do batch, Size somente no oversize e len(encoded) antes de framing, desligando UseCachedSize. Grande 331,488→333,030 ms, CPU 363,687→365,917; sem redução de allocations/payload. O marshaler gerado ainda faz seu próprio Size para prealocar: não elimina uma passagem real comparado ao cache do baseline. Revertido no caminho gerado; será reavaliado separadamente com protowire direto.

[Dados brutos](benchmarks/ggpb_c4-size.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.885 | 1.014 | 0.40 | 3574 | 22026 | 0.896 | 0.485 |
| 10000 | 33.855 | 37.103 | 1.62 | 77091 | 2199660 | 39.904 | 32.763 |
| 100000 | 333.030 | 365.917 | 12.34 | 739357 | 22070295 | 376.398 | 324.551 |
| empty | 0.175 | 0.266 | 0.07 | 530 | 67 | 0.171 | 0.015 |

### C3 — marshal com protowire oficial (KEEP)

Codec específico para batches produzidos pelo encoder, sobre slots reutilizados de C1; não duplica traversal nem monta objetos por scalar. Usa google.golang.org/protobuf/encoding/protowire. Emit continua oferecendo pb.Batch; EmitEncoded oferece bytes do mesmo Batch, sem framing, para adapters que possam consumir wire diretamente. Borrow/copy explícito. Grande 331,488→260,279 ms (−21,5%), CPU 363,687→294,540; wire byte a byte idêntico ao marshal determinístico oficial nas seis fixtures. Complexidade adicional ~180 linhas e manutenção de tags; aceito por ganho material. Size ainda presente nessa medição para separar D2.

[Dados brutos](benchmarks/ggpb_c3-wire.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.795 | 0.928 | 0.44 | 3595 | 22026 | 0.815 | 0.423 |
| 10000 | 27.517 | 30.511 | 1.67 | 77112 | 2199660 | 42.983 | 27.451 |
| 100000 | 260.279 | 294.540 | 12.39 | 739384 | 22070295 | 376.970 | 255.849 |
| empty | 0.191 | 0.260 | 0.07 | 539 | 67 | 0.166 | 0.019 |

### D2 — sem Size no hot path direto (KEEP)

Batches usam o limite conservador pré-interning já existente; somente um envelope excepcional cujo bound ultrapassa 4 MiB exige Size exato antes do callback. Header validado/limitado antes de alocar e End escalar. EmitEncoded verifica len(payload) antes de enviar/framing. Grande C3→D2: 260,279→178,254 ms (−31,5%); CPU 294,540→210,229. Payload idêntico. Ao contrário de D1, aqui foi eliminada uma passagem real de reflection/Size. Retenção segue bounded; hard limit é independente do target.

[Dados brutos](benchmarks/ggpb_c4-wire-size.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.541 | 0.667 | 0.32 | 2507 | 22026 | 0.877 | 0.362 |
| 10000 | 18.032 | 21.499 | 1.54 | 76025 | 2199660 | 39.337 | 17.901 |
| 100000 | 178.254 | 210.229 | 12.26 | 738292 | 22070295 | 372.788 | 179.830 |
| empty | 0.036 | 0.099 | 0.01 | 49 | 67 | 0.185 | 0.017 |

### E — matriz de batches (KEEP default / REJECT maiores)

Alvos 64/128/256/512/1024/2048 KiB, com cap de 256 partes e com cap experimental de 4096. Cache de slots continuou limitado a 256, tables mantiveram limites; cada variante medida nas quatro escalas cold/resident, três repetições. [Todos os dados](benchmarks/ggpb_batch_matrix.jsonl). Com 256 partes o cap de records domina e mudar alvo não muda os bytes: nenhum ganho consistente justifica alterar o default. Aumentar o cap introduz objetos além do cache e esgota a tabela de endpoints, com fallback inline; piora tempo/alloc/payload. Mantidos 256 partes/256 KiB como antes, adequados para backpressure e mensagens pequenas no futuro gRPC. Nenhuma opção experimental entrou no produto.

| Cap partes | Target KiB | Encode grande ms | CPU ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | HeapAfter MiB |
|---|---|---|---|---|---|---|---|---|
| 256 | 64 | 178.676 | 210.746 | 12.26 | 738289 | 22070295 | 375.470 | 3.63 |
| 256 | 128 | 178.378 | 210.403 | 12.27 | 738297 | 22070295 | 374.095 | 3.62 |
| 256 | 256 | 179.948 | 212.500 | 12.26 | 738291 | 22070295 | 377.067 | 3.64 |
| 256 | 512 | 178.854 | 210.224 | 12.26 | 738292 | 22070295 | 373.045 | 3.63 |
| 256 | 1024 | 175.490 | 207.151 | 12.27 | 738297 | 22070295 | 373.832 | 3.63 |
| 256 | 2048 | 178.966 | 211.275 | 12.27 | 738298 | 22070295 | 377.050 | 3.62 |
| 4096 | 64 | 223.948 | 290.689 | 174.26 | 1919833 | 22009602 | 371.473 | 2.19 |
| 4096 | 128 | 272.291 | 406.705 | 302.02 | 2850471 | 22154725 | 377.485 | 2.01 |
| 4096 | 256 | 325.108 | 570.606 | 366.13 | 3315691 | 22403986 | 382.023 | 2.90 |
| 4096 | 512 | 332.904 | 545.311 | 400.92 | 3699129 | 23820940 | 428.329 | 3.32 |
| 4096 | 1024 | 360.570 | 713.372 | 408.64 | 3828849 | 24581109 | 431.834 | 6.23 |
| 4096 | 2048 | 368.129 | 729.538 | 408.63 | 3828846 | 24581109 | 431.410 | 6.23 |

HeapAfter é heap alocado no término, incluindo lixo ainda não coletado, não peak live. Amostragem de heap vivo/batches é qualificada separadamente fora dos tempos.

### f-repeat2-min0 — dictionary heuristic (REJECT)

Protótipo bounded promove símbolos após repetição (2 ou 4 ocorrências); min4 ignora símbolos curtos. Matriz nas quatro escalas. Nenhum ganho relevante de CPU/encode, payload cresce; custo de contagem adicional. Revertido; dictionary imediato e inline baseline permanecem.

[Dados brutos](benchmarks/ggpb_f-repeat2-min0.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.462 | 0.565 | 0.32 | 2512 | 22068 | 0.778 | 0.381 |
| 10000 | 18.481 | 21.514 | 1.54 | 76030 | 2201716 | 39.739 | 17.810 |
| 100000 | 179.065 | 211.318 | 12.27 | 738302 | 22090703 | 375.381 | 179.579 |
| empty | 0.030 | 0.105 | 0.01 | 49 | 67 | 0.181 | 0.020 |

### f-repeat4-min0 — dictionary heuristic (REJECT)

Protótipo bounded promove símbolos após repetição (2 ou 4 ocorrências); min4 ignora símbolos curtos. Matriz nas quatro escalas. Nenhum ganho relevante de CPU/encode, payload cresce; custo de contagem adicional. Revertido; dictionary imediato e inline baseline permanecem.

[Dados brutos](benchmarks/ggpb_f-repeat4-min0.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.527 | 0.641 | 0.32 | 2512 | 22152 | 0.883 | 0.371 |
| 10000 | 18.450 | 21.572 | 1.54 | 76030 | 2205828 | 39.779 | 18.334 |
| 100000 | 181.322 | 213.086 | 12.27 | 738302 | 22131519 | 373.641 | 179.371 |
| empty | 0.035 | 0.109 | 0.01 | 49 | 67 | 0.191 | 0.017 |

### f-repeat2-min4 — dictionary heuristic (REJECT)

Protótipo bounded promove símbolos após repetição (2 ou 4 ocorrências); min4 ignora símbolos curtos. Matriz nas quatro escalas. Nenhum ganho relevante de CPU/encode, payload cresce; custo de contagem adicional. Revertido; dictionary imediato e inline baseline permanecem.

[Dados brutos](benchmarks/ggpb_f-repeat2-min4.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.562 | 0.672 | 0.32 | 2509 | 22482 | 0.800 | 0.377 |
| 10000 | 18.443 | 21.772 | 1.54 | 76027 | 2244604 | 40.532 | 18.688 |
| 100000 | 179.387 | 211.107 | 12.26 | 738293 | 22519595 | 378.650 | 183.053 |
| empty | 0.029 | 0.089 | 0.01 | 49 | 67 | 0.170 | 0.022 |

### E — geometria e heap vivo

Amostras com GC dentro do callback, fora dos tempos, nas quatro escalas. Metadata vazia nesta prova: bytes diferem levemente da campanha territory. [Dados](benchmarks/ggpb_batch_geometry.jsonl). Retenção medida com graph/subgraph vivos; pico amostrado não cobre todos os transientes.

| Cap partes | Target KiB | Batches grande | Frame máximo B | Auxiliar vivo amostrado KiB |
|---|---|---|---|---|
| 256 | 64 | 2110 | 12829 | 349.9 |
| 256 | 128 | 2110 | 12829 | 349.9 |
| 256 | 256 | 2110 | 12829 | 349.9 |
| 256 | 512 | 2110 | 12829 | 355.2 |
| 256 | 1024 | 2110 | 12829 | 349.9 |
| 256 | 2048 | 2110 | 12829 | 349.9 |
| 4096 | 64 | 1289 | 20730 | 562.0 |
| 4096 | 128 | 645 | 41480 | 1119.5 |
| 4096 | 256 | 323 | 82980 | 2262.2 |
| 4096 | 512 | 162 | 165930 | 4495.2 |
| 4096 | 1024 | 132 | 204830 | 5570.3 |
| 4096 | 2048 | 132 | 204830 | 5570.3 |

### H — parallel encoding (REJECT), queries concorrentes (KEEP capacidade existente)

Protótipo com 1/2/4/8 workers e 1/2/4/8 queries independentes sobre o mesmo snapshot aberto. Janela de jobs/completions de 2×workers, emissão ordenada, mesma saída. Para respeitar ownership, workers recebem proto.Clone do batch e retornam cópia do wire; cada worker usa o mesmo codec protowire otimizado. Cópias e GC contam no tempo/CPU. Não é uma comparação com marshal antigo nem zero-copy inseguro.

Três ondas por combinação, nas quatro escalas; [192 amostras](benchmarks/ggpb_concurrency.jsonl). Cada query tem scratch próprio e é executada novamente dentro da onda. Wall de uma onda não é a soma das latências individuais; CPU é user+system agregado do processo. Consumer/payload não mudam com workers, pois o wire é idêntico. Testes com determinismo/typed/error/cancellation passaram. Harness opt-in permanece somente em testes, nenhum worker/flag/pool entra no produto.

| Workers/query | Queries simultâneas | Onda grande ms | Queries/s | Encode latência mediana ms | CPU/query ms | TotalAlloc/query MiB | Allocs/query |
|---|---|---|---|---|---|---|---|
| 1 | 1 | 193.537 | 5.17 | 166.294 | 198.597 | 12.24 | 738279 |
| 1 | 2 | 199.125 | 10.04 | 170.003 | 204.253 | 12.24 | 738278 |
| 1 | 4 | 232.480 | 17.21 | 200.267 | 234.031 | 12.24 | 738277 |
| 1 | 8 | 306.505 | 26.10 | 257.846 | 300.342 | 12.24 | 738276 |
| 2 | 1 | 591.966 | 1.69 | 565.138 | 820.395 | 286.05 | 6259624 |
| 2 | 2 | 725.857 | 2.76 | 698.025 | 985.321 | 286.06 | 6259635 |
| 2 | 4 | 987.863 | 4.05 | 953.469 | 1220.265 | 286.06 | 6259666 |
| 2 | 8 | 1349.207 | 5.93 | 1301.311 | 1452.974 | 286.06 | 6259646 |
| 4 | 1 | 585.319 | 1.71 | 558.471 | 808.826 | 286.20 | 6259674 |
| 4 | 2 | 722.411 | 2.77 | 694.437 | 988.198 | 286.21 | 6259693 |
| 4 | 4 | 961.227 | 4.16 | 920.059 | 1186.422 | 286.21 | 6259707 |
| 4 | 8 | 1320.201 | 6.06 | 1266.057 | 1424.971 | 286.21 | 6259694 |
| 8 | 1 | 592.149 | 1.69 | 564.263 | 824.479 | 286.50 | 6259774 |
| 8 | 2 | 733.218 | 2.73 | 704.338 | 997.762 | 286.50 | 6259786 |
| 8 | 4 | 927.587 | 4.31 | 888.969 | 1135.514 | 286.50 | 6259812 |
| 8 | 8 | 1257.648 | 6.36 | 1211.468 | 1376.669 | 286.50 | 6259787 |

O clone domina o pipeline paralelo: uma query passa de ~166 ms encoding/12 MiB alocados para ~559–565 ms/286 MiB. Nem 8 workers recuperam esse custo; com 8 queries o caminho sequencial entrega ~26 queries/s, contra ~6 no paralelo. Rejeitado. Distribuir diretamente a materialização exigiria mudar a montagem/particionamento de batches para preservar exatamente os bytes e lidar com continuations; sem benefício deste protótipo, não foi introduzido outro scheduler/segundo traversal. A capacidade existente de queries concorrentes escala melhor neste workload. Não se extrapola throughput do sintético para produção.

### G — caches de strings entre batches (REJECT)

Cache bounded de StringID persistiu entre batches; endpoint cache bounded teve limpeza por capacidade, independente do dictionary local. Grande: 178,254→172,318 ms (−3,3%), contra 175,187 ms na requalificação final; TotalAlloc 12,26→12,21 MiB (−0,4%), allocations 738.292→729.336 (−1,2%). Sem benefício relevante e com nova política de eviction, revertido. A/B já removem as chamadas repetidas mais importantes. Value.Text é apenas accessor: a cópia mmap→string ocorre em PropertyIterator.Next/Data.String, não no accessor. Nenhuma nova API de bytes/unsafe foi adicionada ao core. O perfil final quantifica a parcela remanescente; não se afirma que ela seja um limite inevitável.

[Dados brutos](benchmarks/ggpb_g-caches.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.523 | 0.634 | 0.32 | 2498 | 22026 | 0.938 | 0.361 |
| 10000 | 17.874 | 20.911 | 1.58 | 75139 | 2199660 | 39.558 | 18.082 |
| 100000 | 172.318 | 204.119 | 12.21 | 729336 | 22070295 | 379.407 | 179.426 |
| empty | 0.034 | 0.121 | 0.01 | 49 | 67 | 0.177 | 0.014 |

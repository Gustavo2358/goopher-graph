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

# Campanha JSON vs GGPB — 2026-10-06

## Consolidação da otimização — 2026-10-06

O formato lógico e a separação do core foram preservados. No mesmo caso de 90k/450k, encoding caiu **510,982→186,220 ms (2,74×)**, CPU do produtor **607,138→217,594 ms (−64,2%)**, TotalAlloc **352,13→12,26 MiB (−96,5%)**, allocations **9.309.134→738.293 (−92,1%)**. Payload **29.914.323→22.067.479 bytes**, −26,2% adicional e **−69,4% versus JSON**. Query permaneceu ~26 ms. Encoding ainda custa ~7,2× a query; não se declara limite inevitável nem custo equivalente à traversal.

Baseline c0 é o profiling/checkpoint feito antes de alterar o hot path (HEAD inicial a6e3d32). A referência da campanha original era JSON 556,872 / GGPB 517,118 ms; a requalificação final JSON ficou 575,186 ms. Variação de coleta/temperatura/GC existe: compare medianas e todos os dados, não um número selecionado. Cinco processos por combinação, mesmos snapshots/queries, ordem rotativa, page cache quente, sem fsync, sem outros gates concorrentes na coleta. CPU cold inclui query+encode; allocs cold também. RSS inclui mmap/open, não é heap exclusivo do encoder.

[300 amostras finais](benchmarks/ggpb_optimized_samples.jsonl), [300 residentes](benchmarks/ggpb_resident.jsonl), [192 ondas concorrentes finais](benchmarks/ggpb_concurrency_final.jsonl), [geometria/heap final](benchmarks/ggpb_geometry_final.jsonl). Os resultados iniciais abaixo foram preservados como histórico, inclusive hipóteses não confirmadas e experimentos rejeitados.

### Campanha final: todos os comandos/casos

| Caso | Nodes | Edges | JSON bytes | GGPB bytes | Redução | JSON encode ms | GGPB encode ms | Speedup | RSS J/G MiB |
|---|---|---|---|---|---|---|---|---|---|
| 100/territory | 90 | 450 | 72103 | 22022 | 69.46% | 0.775 | 0.459 | 1.69× | 5.02 / 5.34 |
| 100/anti-territory | 90 | 450 | 72108 | 22027 | 69.45% | 0.671 | 0.529 | 1.27× | 5.08 / 5.28 |
| 100/between | 90 | 450 | 72119 | 22032 | 69.45% | 0.660 | 0.555 | 1.19× | 5.13 / 5.29 |
| 100/between vazio | 0 | 0 | 178 | 67 | 62.36% | 0.050 | 0.035 | 1.40× | 5.01 / 4.79 |
| 10000/territory | 9000 | 45000 | 7212557 | 2199378 | 69.51% | 58.405 | 19.446 | 3.00× | 14.33 / 11.16 |
| 10000/anti-territory | 9000 | 45000 | 7212562 | 2199383 | 69.51% | 58.960 | 19.033 | 3.10× | 14.37 / 11.16 |
| 10000/between | 9000 | 45000 | 7212573 | 2199388 | 69.51% | 58.224 | 19.089 | 3.05× | 14.55 / 11.30 |
| 100000/territory | 90000 | 450000 | 72214059 | 22067479 | 69.44% | 575.186 | 186.220 | 3.09× | 57.32 / 56.54 |
| 100000/anti-territory | 90000 | 450000 | 72214064 | 22067484 | 69.44% | 574.236 | 185.808 | 3.09× | 57.46 / 56.69 |
| 100000/between | 90000 | 450000 | 72214075 | 22067489 | 69.44% | 574.929 | 185.264 | 3.10× | 57.41 / 56.91 |

### Query, serialization, total e consumidor finais

| Caso | Query J/G ms | Serialization J/G ms | Total J/G ms | CPU J/G ms | Consumer J/G ms | Consumer RSS J/G MiB |
|---|---|---|---|---|---|---|
| 100/territory | 0.041 / 0.039 | 0.775 / 0.459 | 0.815 / 0.492 | 0.912 / 0.561 | 1.299 / 0.859 | 5.16 / 5.47 |
| 100/anti-territory | 0.042 / 0.042 | 0.671 / 0.529 | 0.714 / 0.571 | 0.785 / 0.637 | 1.394 / 0.828 | 5.15 / 5.51 |
| 100/between | 0.049 / 0.059 | 0.660 / 0.555 | 0.709 / 0.614 | 0.787 / 0.680 | 1.350 / 0.836 | 5.16 / 5.48 |
| 100/between vazio | 0.029 / 0.029 | 0.050 / 0.035 | 0.082 / 0.064 | 0.120 / 0.106 | 0.050 / 0.161 | 4.85 / 5.13 |
| 10000/territory | 2.670 / 2.678 | 58.405 / 19.446 | 61.213 / 22.206 | 64.066 / 22.689 | 117.574 / 39.679 | 9.83 / 10.67 |
| 10000/anti-territory | 2.779 / 2.776 | 58.960 / 19.033 | 61.740 / 21.846 | 64.363 / 22.184 | 116.657 / 39.481 | 9.65 / 10.57 |
| 10000/between | 4.120 / 4.200 | 58.224 / 19.089 | 62.337 / 23.327 | 65.006 / 23.666 | 117.028 / 39.674 | 9.80 / 10.84 |
| 100000/territory | 25.973 / 25.703 | 575.186 / 186.220 | 601.663 / 212.207 | 628.780 / 217.594 | 1155.944 / 371.718 | 10.08 / 11.11 |
| 100000/anti-territory | 27.020 / 26.707 | 574.236 / 185.808 | 601.154 / 212.426 | 628.664 / 217.939 | 1160.170 / 368.876 | 10.07 / 10.84 |
| 100000/between | 40.502 / 40.500 | 574.929 / 185.264 | 615.337 / 225.783 | 643.327 / 231.593 | 1161.117 / 373.175 | 10.05 / 10.99 |

### Baseline c0 versus final, quatro escalas

| Caso | Encode c0/final ms | CPU c0/final ms | TotalAlloc c0/final MiB | Allocs c0/final | Payload c0/final | Consumer c0/final ms |
|---|---|---|---|---|---|---|
| 100 | 0.891 / 0.459 | 1.021 / 0.561 | 0.51 / 0.32 | 10414 / 2507 | 29850 / 22022 | 0.957 / 0.859 |
| 10000 | 59.022 / 19.446 | 70.807 / 22.689 | 35.32 / 1.54 | 931932 / 76025 | 2984088 / 2199378 | 49.045 / 39.679 |
| 100000 | 510.982 / 186.220 | 607.138 / 217.594 | 352.13 / 12.26 | 9309134 / 738293 | 29914323 / 22067479 | 422.519 / 371.718 |
| empty | 0.185 / 0.035 | 0.270 / 0.106 | 0.07 / 0.01 | 529 / 49 | 67 / 67 | 0.165 / 0.161 |

### Engine residente

Snapshot aberto uma vez, warmup, cinco novas queries+encodings por formato/HEAD no mesmo processo. Runtime e descritores aquecidos; mapas/buffers são privados e reutilizados dentro de cada encoding, sem pool global entre queries. Query e CPU dela são separadas. TotalAlloc abaixo é somente encode. Os três comandos foram medidos nas três escalas e o vazio. Não há promise de cold cache ou SLA.

| Caso | Query final ms | Encode c0/final ms | CPU encode c0/final ms | Total query+encode c0/final ms | Alloc encode c0/final MiB |
|---|---|---|---|---|---|
| 100/territory | 0.027 | 0.656 / 0.384 | 0.696 / 0.414 | 0.687 / 0.411 | 0.38 / 0.31 |
| 100/anti-territory | 0.027 | 0.643 / 0.370 | 0.685 / 0.401 | 0.677 / 0.397 | 0.38 / 0.31 |
| 100/between | 0.040 | 0.634 / 0.385 | 0.676 / 0.411 | 0.675 / 0.425 | 0.38 / 0.31 |
| 100/empty | 0.019 | 0.017 / 0.013 | 0.026 / 0.020 | 0.037 / 0.032 | 0.00 / 0.00 |
| 10000/territory | 2.534 | 51.174 / 18.816 | 58.399 / 18.867 | 53.667 / 21.365 | 35.03 / 1.39 |
| 10000/anti-territory | 2.639 | 50.989 / 18.860 | 58.104 / 18.893 | 53.603 / 21.479 | 35.03 / 1.39 |
| 10000/between | 3.908 | 51.151 / 19.831 | 58.665 / 19.856 | 55.018 / 23.813 | 35.03 / 1.39 |
| 100000/territory | 25.553 | 509.167 / 188.709 | 573.395 / 191.193 | 534.541 / 214.104 | 349.99 / 10.26 |
| 100000/anti-territory | 26.208 | 508.142 / 186.533 | 575.150 / 188.957 | 534.214 / 212.925 | 350.00 / 10.26 |
| 100000/between | 39.358 | 509.821 / 187.892 | 577.125 / 191.099 | 550.500 / 226.725 | 350.00 / 10.26 |

### Concorrência requalificada após B2

| Workers/query | Queries | Onda grande ms | Queries/s | Encode latência mediana ms | CPU/query ms | TotalAlloc/query MiB |
|---|---|---|---|---|---|---|
| 1 | 1 | 199.454 | 5.01 | 172.870 | 204.364 | 12.24 |
| 1 | 2 | 207.006 | 9.66 | 178.570 | 212.085 | 12.24 |
| 1 | 4 | 242.850 | 16.47 | 205.578 | 241.291 | 12.24 |
| 1 | 8 | 320.523 | 24.96 | 259.080 | 301.493 | 12.24 |
| 2 | 1 | 576.161 | 1.74 | 549.101 | 786.616 | 286.08 |
| 2 | 2 | 704.330 | 2.84 | 676.457 | 952.771 | 286.08 |
| 2 | 4 | 982.269 | 4.07 | 943.835 | 1213.098 | 286.08 |
| 2 | 8 | 1340.893 | 5.97 | 1290.039 | 1451.453 | 286.08 |
| 4 | 1 | 590.958 | 1.69 | 563.253 | 819.326 | 286.22 |
| 4 | 2 | 724.092 | 2.76 | 693.789 | 989.538 | 286.23 |
| 4 | 4 | 955.438 | 4.19 | 915.057 | 1176.917 | 286.23 |
| 4 | 8 | 1320.592 | 6.06 | 1267.435 | 1436.149 | 286.23 |
| 8 | 1 | 590.119 | 1.69 | 562.192 | 814.238 | 286.52 |
| 8 | 2 | 722.635 | 2.77 | 693.587 | 994.102 | 286.52 |
| 8 | 4 | 921.117 | 4.34 | 887.322 | 1136.594 | 286.53 |
| 8 | 8 | 1258.711 | 6.36 | 1207.482 | 1375.335 | 286.52 |

### Caso adverso: endpoints dispersos

Fixture adicional com a mesma forma/count/properties, sem hubs e IDs de edges permutados; ring por componente garante que territory retorna 90k/450k. Generator reproduzível `tools/ggpb_scatter.py`. Não substitui fixtures existentes. Cinco processos por variante; ordem dos checkpoints sequencial, portanto pequenas diferenças de tempo não são tratadas como significativas. [Dados](benchmarks/ggpb_scatter.jsonl).

| Variante | Encode ms | CPU query+encode ms | TotalAlloc MiB | Allocs | Payload B | Consumer ms |
|---|---|---|---|---|---|---|
| json | 573.692 | 649.896 | 120.18 | 9360206 | 72214059 | 1149.882 |
| c0 | 549.268 | 674.864 | 352.13 | 9309136 | 29914323 | 419.721 |
| c4 | 252.993 | 318.725 | 24.60 | 1540321 | 32370181 | 403.724 |
| adaptive | 252.440 | 316.008 | 24.59 | 1540320 | 29900969 | 419.155 |

C4 mostra o resultado negativo: endpoint dictionary incondicional +8,2% bytes.
B2/adaptive evita a expansão e mantém o wire autossuficiente. No corpus original,
contar endpoints acrescentou ~5% encode/4% CPU, explicitamente aceito por essa
robustez. O disperso não ganhou mais tamanho contra o GGPB original, embora
encoding/CPU/allocs tenham caído substancialmente. Não afirmar que refs sempre
compactam qualquer distribuição.

### Perfis antes/depois e interpretação

Perfis CPU/mem reais (gzip pprof) e summaries estão em
[ggpb_profiles](benchmarks/ggpb_profiles/baseline-ggpb.cpu.pprof).
[CPU final completo](benchmarks/ggpb_profiles/final-ggpb-cpu-full.txt),
[alloc_space final](benchmarks/ggpb_profiles/final-ggpb-alloc_space.txt),
[alloc_objects final](benchmarks/ggpb_profiles/final-ggpb-alloc_objects.txt),
[heap após GC](benchmarks/ggpb_profiles/final-ggpb-inuse_space.txt).
JSON e inline também têm os dois perfis/summaries.

| Hotspot | Baseline CPU cumulativa | Final CPU cumulativa | Interpretação |
|---|---|---|---|
| mallocgc | 23,9% | 4,42% (predomina tiny allocator) | Slots/reuso eliminam milhões de objetos |
| MessageInfo.marshalAppendPointer | 21,3% | não aparece | Codec protowire sobre slots |
| MessageInfo.sizePointer | 14,4% | não aparece | Limite conservador e check final |
| wireEncoder.batch | inexistente | 26,8% | Append tags/scalars/cópias agora explícitos |
| mapaccess2_fast32 | parcela menor | 18,6% | Caches/símbolos/count/ref endpoints |
| Data.String | 5,85% alloc_space; 15,7% alloc_objects | 92,0% alloc_space; ~99% alloc_objects | Restaram sobretudo IDs externos obrigatórios |
| CRC32 | não dominante | 1,26% CPU | Não explica centenas de ms |
| writeAll | sink Discard | 0,32% CPU | Perfil exclui syscalls de arquivo |

Cumulativas têm sobreposição. CPU baseline: seis encodings após warmup; final:
18 encodings para obter duração semelhante (~3 s). Memprofile inclui warmup,
open/query: sete versus 19 encodings, com sampling padrão. Compare shares e os
counters medidos, não totais brutos dos profiles. Data.String caiu de ~20,7 para
~9,5 MiB amostrados por encoding; tornou-se dominante nas alocações porque a
árvore temporária foi removida, não por ter crescido. Heap/profile após GC contém
principalmente runtime/profiler, não representa pico. Geometria/live retention
foi medida separadamente no callback com graph/subgraph mantidos vivos.

A hipótese central foi parcialmente corrigida: **a maior parte dos ~500 ms não
era cópia de strings isoladamente; objetos/slices Protobuf e reflection/Size
consumiam muito mais**. StringID/ref caches ajudam, mas o ganho grande veio de
reuso e wire append. Value.Text apenas retorna a string já formada pelo iterator.
Nenhum unsafe, nova API de bytes ou acesso a colunas privadas foi introduzido.
Não se demonstrou que 186 ms seja um piso inevitável. Ainda há trabalho por
campo/ID, maps, append de varints e cópias para 22 MB de dados semânticos.

### Decisões finais e limites

| Experimento | Decisão | Motivo |
|---|---|---|
| A StringID cache/dictionary | KEEP | ~5% encode e menos resoluções/alocações |
| B endpoints locais | KEEP com B2 | Compacta localidade, não exige índice global |
| B2 count + comparação de custo | KEEP | Evita +8,2% payload disperso, custa ~5% encode normal |
| C1 objetos/slices por query | KEEP | ~90% menos bytes alocados nesse checkpoint |
| C2 maps/Records/buffers | KEEP | ~64% menos alloc adicionais, ~5% encode |
| C3 protowire sobre slots | KEEP | ~21% encode, wire idêntico, custo de manutenção explícito |
| D1 sem Size externo no marshal gerado | REJECT | Marshal ainda faz Size, sem ganho |
| D2 sem Size no codec direto | KEEP | ~31% encode adicional |
| E 64 KiB..2 MiB | KEEP default | 256 partes domina; targets não mudam bytes |
| E 4096 partes | REJECT | Piora CPU/alloc/payload; não escolher batches grandes |
| F inline / repetição 2/4 / frequência | KEEP imediato | Heurísticas não melhoram; inline baseline perde bytes |
| G caches cross-batch | REJECT | ~0,4% menos bytes alocados, efeito pequeno de tempo |
| H 2/4/8 workers | REJECT | Clone de ownership domina; throughput pior |
| Queries concorrentes | KEEP existente | Escala melhor sem workers internos |

O codec direto acrescenta manutenção de tags e nested sizes; schema gerado é a
fonte da verdade e testes diferenciais/fuzz exigem bytes idênticos ao marshal
oficial. Não há uma segunda traversal/direct graph implementation divergente:
slots usados por Emit e EmitEncoded são os mesmos. Nada de worker/pool global,
cache proporcional ao resultado, mmap compartilhado com consumer ou dependência
de arquivo/seek no modelo lógico.

**Para o próximo passo gRPC:** protocolo/stream são bons candidatos a um PoC,
com engine residente e consumidor real. Os 186 ms medem EmitEncoded + framing.
Emit + codec Protobuf padrão também recebe o reuso e dictionaries, mas o
checkpoint com marshal gerado ficou ~331–333 ms. Um gRPC padrão não herda
automaticamente o ganho de protowire; deve-se medir codec padrão versus adapter
que envie os payloads canônicos de EmitEncoded. Buffers/mensagens são borrowed
até callback retornar; retenção assíncrona exige cópia. Não presumir ownership
pós-Send. Não foi implementado gRPC ou medido transporte/Lambda.

Cache de slots conserva apenas pequenas partes (até 16 properties/labels de
capacidade); distribuições com entidades muito largas podem ter alocações maiores.
Escalares/framing mantêm limites de 1/4 MiB. Reader valida estrutura/tipos/counts,
mas não guarda índice global para validar membership de endpoints. RSS tem piso
de mmap e caiu pouco mesmo com TotalAlloc 96,5% menor. Consumer vazio continua
pagando inicialização Protobuf; ganhos no produtor não eliminam esse custo.
Corpus público sintético/local, sem garantia para toda distribuição nem SLA.

### Reproduzir otimização/profiling/residência

```sh
go build -o bin/encodingmeasure ./tools/encodingmeasure
go build -o bin/resultmeasure ./tools/resultmeasure
bin/encodingmeasure --snapshot /tmp/ggpb-campaign/100000/graph.snapshot \
  --format ggpb --warmup 1 --repeats 18 \
  --cpuprofile /tmp/ggpb.cpu --memprofile /tmp/ggpb.mem
go tool pprof -top /tmp/ggpb.cpu
go tool pprof -top -sample_index=alloc_space /tmp/ggpb.mem
go tool pprof -top -sample_index=alloc_objects /tmp/ggpb.mem
bin/encodingmeasure --snapshot /tmp/ggpb-campaign/100000/graph.snapshot \
  --format ggpb --query-each --query territory --repeats 5 --output /tmp/resident.ggpb
python3 tools/ggpb_checkpoints.py --bin "$PWD/bin" --fixtures /tmp/ggpb-campaign \
  --work /tmp/ggpb-checkpoint --checkpoint final --repeats 5
GGPB_EXPERIMENT_FIXTURES=/tmp/ggpb-campaign GGPB_EXPERIMENT_OUTPUT=/tmp/concurrency.jsonl \
  go test ./ggpb -run '^TestConcurrencyExperiment$' -count=1 -timeout 20m
GGPB_GEOMETRY_FIXTURES=/tmp/ggpb-campaign GGPB_GEOMETRY_OUTPUT=/tmp/geometry.jsonl \
  go test ./ggpb -run '^TestBatchGeometryExperiment$' -count=1
python3 tools/ggpb_scatter.py --nodes 100000 --output /tmp/ggpb-scatter
bin/gophergraph build --nodes /tmp/ggpb-scatter/nodes --edges /tmp/ggpb-scatter/edges \
  --output /tmp/ggpb-scatter/graph.snapshot
```

E/F/G são protótipos temporários revertidos; history/checkpoint notes explicam as
alterações isoladas de constantes/policy. E variou batchTarget (64..2048 KiB) e
maxParts (256/4096), mantendo cache de slots 256; F promoveu símbolo na 2ª/4ª
ocorrência ou apenas com len>=4 usando mapa bounded; G reteve cache de símbolos
entre batches e limpou cache de endpoints por capacidade. Todos os dados foram
salvos antes de restaurar o produto, sem código morto no hot path final.


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

### B2 — endpoints somente quando economizam bytes (KEEP)

Caso adicional determinístico sem hubs/localidade: mesmos 90k/450k retornados, ring garante conectividade, targets pseudoaleatórios e IDs de edges permutados. B incondicional expandiu 29.914.323→32.370.181 bytes (+8,2%) versus v1 textual. Protótipo conta endpoints no batch (arrays/map bounded 512) antes da emissão e compara custo wire da tabela/ref contra inline. Disperso voltou a 29.900.969 bytes, encode 252,993→252,440 ms; sem ganhos artificiais. No corpus original encode 178,254→187,253 ms (+5,0%), CPU 210,229→218,334 (+3,9%); payload 22.070.295→22.067.479. Mantido apesar do custo adicional para evitar expansão sistemática em workloads dispersos. Não altera .proto nem framing; refs continuam locais/opacos, schema permite fallback inline. Teste de expectativa de todos os endpoints numéricos atualizado para a política legítima e fortalecido com comparação ao Graph; novos testes independentes exigem singleton inline/repetido referenciado.

[Dados brutos](benchmarks/ggpb_b-adaptive.jsonl). Medianas de três execuções; CPU/alloc cold incluem query. Resident abaixo mede somente encode, após warmup.

| Caso | Encode cold ms | CPU cold ms | TotalAlloc MiB | Allocs | Payload bytes | Consumer ms | Encode residente ms |
|---|---|---|---|---|---|---|---|
| 100 | 0.523 | 0.642 | 0.32 | 2507 | 22022 | 0.896 | 0.389 |
| 10000 | 19.313 | 22.370 | 1.54 | 76025 | 2199378 | 39.469 | 19.519 |
| 100000 | 187.253 | 218.334 | 12.27 | 738297 | 22067479 | 370.349 | 187.661 |
| empty | 0.034 | 0.097 | 0.01 | 49 | 67 | 0.188 | 0.014 |

### Gates finais executados

- `go test -count=1 ./...`: passou, incluindo E2E/arquitetura/WASM e paridade JSON.
- `CGO_ENABLED=1 go test -race -count=1 ./...`: passou, inclusive runtime WASM completo (~237 s).
- `go vet ./...`, gofmt, `CGO_ENABLED=0 go build ./cmd/gophergraph`, `git diff --check`: passaram.
- FuzzReader 10.000, FuzzWireScalar 10.000, FuzzNeptuneCSV 10.000, FuzzSnapshotDecode 10.000, FuzzModuleAdmission 10.028: passaram. O nome WASM foi conferido e o target real foi executado; uma tentativa anterior sem match não conta como gate.
- `go test -run '^$' -bench . -benchmem -benchtime=1x ./...`: passou. Uma iteração não substitui a campanha de cinco processos.
- TestBoundedMemory: 1k/100k nodes+edges, 2.137.046/213.710.194 bytes descartados, heap vivo auxiliar amostrado 5.944/724.688 bytes; tolerância existente 8 MiB e limite de 256 KiB antes da primeira escrita inalterados. Sem acumular o resultado.
- Cinco hashes iguais por combinação em toda campanha final; testes concorrentes de determinismo e diferencial byte a byte com marshaler oficial passaram.
- Schema Go/Python regenerado; Python oficial validou arquivos novos typed/numeric/presence, grande normal e disperso. `decode` dos dois grandes comparado por `cmp` ao JSON direto (~72 MB cada), idêntico.
- Checker de documentos/spec passou; é complemento aos testes Go, não substituto da engine.

Sem relaxar fixtures, goldens ou limites existentes. O golden vazio continua igual; política de endpoint ganhou testes explícitos singleton/repetido e paridade completa ao Graph. Apenas dados sintéticos públicos/locais foram usados.

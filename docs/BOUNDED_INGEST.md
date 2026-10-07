# Ingestão com memória limitada

## Desenho e compatibilidade

A CLI usa `ingest.Build` com `Options.Scratch` e `MemoryBudget`. A biblioteca
preserva `Options{}` como caminho heap para compatibilidade e como comparação
nos testes. Configurar somente MemoryBudget, sem Scratch, é erro de uso.
Snapshot v1, Graph, queries e publicação não mudaram.

O pipeline externo faz estes passes:

1. O loop compartilhado lê cada fonte uma vez, conserva rejeições, warnings e
   contadores e grava contribuições de identidade, label e propriedade em scratch.
2. Ordena por ID, tipo de contribuição, chave e valor canônico. Runs usam arena e
   descritores com capacidade fixa; merges de dois runs mantêm uma pilha de até
   64 níveis. Não há lista de runs ou número de arquivos proporcional aos registros.
3. Consolida cada ID em streams, mantendo contagem e as duas menores origens.
   Cada contribuição de propriedade conserva uma sequência atribuída antes do
   sort. First/last-wins de propriedades single de nodes escolhe a menor/maior
   sequência, conservando só um candidato e sua origem; a ordem por valor dos
   runs não determina o vencedor. Duplicatas mantêm suas sequências.
   Identidades de edge vêm antes das propriedades: quarentena elimina todo o ID
   e seus conflitos de propriedades. Para um grupo de propriedade, primeiro decide
   conflitos; aceita só o vencedor quando configurado ou relê o intervalo
   aceito, deduplicando valores consecutivos.
   Nem mesmo um único node, set, label ou hub enorme exige buffering do grupo.
4. Depois de todos os nodes, cria um índice de IDs em arquivos mmap (offsets e
   bytes). Busca binária resolve endpoints; ausências rejeitam cada registro de
   edge antes do staging. Só então lê e consolida edges.
5. Ordena/deduplica strings em disco. Busca nos dicionários mmap transforma tokens
   aceitos em colunas LE escritas sequencialmente. Ordenações externas separadas
   produzem CSR forward/reverse e postings de labels/propriedades.
6. Mapeia as colunas finais e valida o Graph. A validação CSR prova unicidade por
   membership, ordenação estrita de `(neighbor,label,edgeID)` e contagem total;
   não aloca bitset de edges. O writer existente serializa/valida/publica o mesmo
   snapshot e preserva leitores de gerações anteriores.

Keys temporárias usam strings terminadas com escaping de NUL e inteiros em BE
para comparação lexical; colunas finais continuam LE. Tipos, bits de floats,
zeros assinados, NaNs canônicos e strings permanecem distintos. Sem hash com
colisões, amostragem ou partições que exijam caber inteiras na RAM.

## Garantias de recursos

O conjunto vivo próprio é **O(orçamento + maior registro/header normalizado +
metadados dos catálogos + opções)**, além de um número fixo de buffers de I/O e
de views mmap. Não depende do total de entidades, strings, propriedades ou
adjacências. Um token maior que a arena gera um run individual; o limite existente
`MaxRecordBytes` continua necessário para controlar esse termo. Decoders customizados
precisam respeitar Limits; sinks de diagnóstico que acumulam eventos adicionam
memória por conta do chamador.

- `--memory-budget`: orçamento dos buffers de sort, padrão 64 MiB, mínimo 1 MiB.
  Não é um teto exato de HeapAlloc, RSS, GC, headers ou diagnósticos.
- `--max-record-bytes`: limite separado de cada registro/header, padrão 64 MiB.
  Para ambientes pequenos, configure ambos. Ex.: orçamento 16 MiB e registro 1 MiB.
- `--temp-dir`: diretório pai existente em disco, padrão diretório do output.
  A CLI rejeita scratch dentro dos catálogos, inclusive aliases por symlink.
  O chamador da biblioteca tem a mesma obrigação ao compor seus ports.
- RSS inclui páginas dos arquivos mapeados; o kernel pode recuperá-las. Sem pressão
  de memória, RSS pode crescer com os bytes mapeados mesmo com heap constante.
  O endereço virtual precisa comportar os mappings em Linux/amd64.
- Workspace privado com diretório 0700 e arquivos 0600. Arquivos e mappings têm
  fechamento explícito. Build remove os nomes temporários antes de retornar;
  o Graph mantém apenas os mappings finais até Close. Nenhum finalizer.
- Não há quota de disco implícita. Scratch pode ocupar várias vezes o input e
  escreve mais bytes que seu pico de ocupação por causa dos merges. Considere também
  o snapshot anterior, o novo staging de publicação e os mappings finais ainda vivos.
- Falhas de criação/escrita/leitura/map/cleanup e falta de espaço são operacionais.
  Nenhum prefixo é anunciado como build bem sucedido. A publicação existente só
  começa depois do build válido. Cancelamento limpa recursos ao retornar.
- SIGKILL/queda não executam cleanup: podem restar `.gophergraph-build-*`. Remova
  somente workspaces de processos encerrados. Não há retomada automática.
- Use armazenamento em disco. Scratch em tmpfs consome RAM e invalida a premissa
  operacional de spill; o builder não contorna quotas nem configura GC global.

## Medições antes/depois

Linux/amd64, Go 1.26.0, Ryzen 5 5600GT, GC padrão, filesystem local ext4. Corpus
sintético do gerador existente: multilabel, propriedades, ciclos, hubs, loops e
paralelas; cinco edges por node, índices `group` e `score`. Nenhum corpus real do
usuário foi fornecido. Execuções isoladas e sequenciais, sem controle de cold cache.

`tools/buildmeasure` amostra HeapAlloc a cada 10 ms. GNU time mede RSS. O perfil
inicial atribuiu cerca de 97% do heap retido visível no último GC às chamadas de
`builder.stage`, incluindo origens e clones de strings. Staging, dicionários, colunas, CSR e memberships coexistiam no caminho
antigo; o pico continuava presente na transição para publicação. O backend externo
não usa essas coleções globais.

| Registros | Backend / orçamento | Pico heap | Pico RSS | Build | Publicação | Snapshot |
|---:|---|---:|---:|---:|---:|---:|
| 600 mil | Heap antigo | 1.348,66 MiB | 1.482,67 MiB | 4,31 s | 0,25 s | 49,21 MiB |
| 1,2 milhão | Heap antigo | 2.685,95 MiB | 3.198,39 MiB | 9,23 s | 0,52 s | 98,42 MiB |
| 600 mil | Externo / 16 MiB | 34,13 MiB | 109,82 MiB | 7,66 s | 0,25 s | 49,21 MiB |
| 1,2 milhão | Externo / 16 MiB | 34,48 MiB | 208,45 MiB | 16,47 s | 0,49 s | 98,42 MiB |
| 2,4 milhões | Externo / 16 MiB | 34,60 MiB | 405,17 MiB | 35,53 s | 0,97 s | 196,84 MiB |

Em 1,2 milhão, leitura/merge leva 8,10 s e canonicalização/CSR/índices 8,37 s.
O heap máximo caiu cerca de 98,7%, com build 1,78× mais lento. O heap após GC com
Graph aberto foi 0,35 MiB no externo e 121,12 MiB no antigo. Isso mede retenção;
o GC final está fora dos tempos de build/publicação e não faz parte do algoritmo.
A alocação acumulada externa ainda cresce com trabalho: 5,14 GiB em 1,2 milhão e
10,98 GiB em 2,4 milhões; não confundir TotalAlloc com heap vivo.

O `cmp` dos snapshots de 1,2 milhão passou. A regressão compara também os 12
corpora adversariais/modelos, relatórios sem duração, diagnósticos completos,
permutações, conflitos entre runs e registro maior que a arena.

| Registros / externo | Pico scratch | Bytes escritos em scratch | Pico mappings de colunas | Máximo de arquivos scratch |
|---:|---:|---:|---:|---:|
| 600 mil | 290,85 MiB | 1,30 GiB | 49,21 MiB | 34 |
| 1,2 milhão | 581,73 MiB | 3,04 GiB | 98,42 MiB | 35 |
| 2,4 milhões | 1.163,45 MiB | 7,00 GiB | 196,84 MiB | 36 |

Scratch não inclui input, snapshot publicado, staging de publicação ou blocos do
filesystem. Durante validação da publicação, coexistem mappings de colunas e do
snapshot serializado; isso explica o RSS sem pressão próximo ao dobro do snapshot.

### Limite real de RAM

Os 2,4 milhões de registros também concluíram num cgroup com `MemoryMax=256 MiB`
e `MemorySwapMax=0`: heap 34,55 MiB, RSS 255,39 MiB, build 62,25 s e publicação
2,65 s. Houve 626 major faults e nenhum swap. O kernel recuperou páginas dos
arquivos; nenhum GOGC/GOMEMLIMIT foi ajustado.

Uma execução adicional completou o mesmo corpus com **MemoryMax=64 MiB**, swap
zero e orçamento de sort de 1 MiB: heap 5,42 MiB, RSS 64,13 MiB, build 90,33 s,
publicação 3,45 s e 1.350 major faults. Os CSVs ocupam cerca de 85 MiB e o snapshot
196,84 MiB, ambos maiores que o limite do cgroup. RSS e cobrança do cgroup são
métricas distintas (por exemplo, páginas compartilhadas); o limite é aplicado
pelo kernel ao cgroup inteiro. O processo concluiu sem ajuste de GC.

Os testes automatizados com orçamento de 1 MiB também passaram: 1,2M/2,4M
registros tiveram picos de 5,58/5,71 MiB e heap vivo abaixo de 0,38 MiB. Um único
node com 240 mil valores String set de 266 bytes, todos indexados, teve pico de
5,25 MiB. Esses testes têm teto fixo de heap em subprocessos e consultas reais.

## Reproduzir

Use diretório dedicado num filesystem em disco com espaço livre:

```sh
mkdir -p bin .measure
CGO_ENABLED=0 go build -o bin/buildmeasure ./tools/buildmeasure
go run ./tools/benchdata --nodes 200000 --output .measure/1200k
/usr/bin/time -v bin/buildmeasure --input .measure/1200k
cp .measure/1200k/graph.snapshot .measure/before.snapshot
/usr/bin/time -v bin/buildmeasure --input .measure/1200k --memory-budget 16777216
cmp .measure/before.snapshot .measure/1200k/graph.snapshot
```

Para 600 mil e 2,4 milhões, use respectivamente `--nodes 100000` e `400000`.
A qualificação com limite real usa um cgroup transitório local, quando disponível:

```sh
systemd-run --user --quiet --wait --pipe \
  -p MemoryMax=268435456 -p MemorySwapMax=0 \
  /usr/bin/time -v "$PWD/bin/buildmeasure" \
  --input "$PWD/.measure/2400k" --memory-budget 16777216
```

Para repetir a prova de 64 MiB, troque `MemoryMax` por `67108864` e
`--memory-budget` por `1048576`.

Os testes de escala automatizados e seus limites de heap estão em
[TESTING](TESTING.md#ingestão-externa). Esses resultados qualificam corpora
sintéticos e o ambiente medido; não estabelecem SLA ou dimensionamento universal
para comprimentos de strings, número de arquivos e propriedades diferentes.

### Sequência de propriedades no spill

A [medição de 2026-10-07](NODE_PROPERTY_CONFLICT_BENCHMARKS.md) compara o
payload atual com sequência somente onde first/last-wins precisa dela.
Em 22 builds grandes, omitir sequência nos demais casos reduziu I/O lógico
total em 1,40–1,94%, sem aceleração consistente. A implementação atual foi
mantida; dados brutos, patch experimental e reprodução estão na nota.

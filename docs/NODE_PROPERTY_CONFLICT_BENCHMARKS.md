# Custo da sequência de propriedades no spill

Medição local de 2026-10-07. A implementação do PR #6 grava oito bytes de
sequência por contribuição de propriedade. A variante experimental conserva
esses bytes somente para `single` de nodes em first/last-wins. Ambas continuam
atribuindo sequência a todas as contribuições, preservando a checagem de overflow.

O patch experimental está em
[tools/conflict_io_selective.patch](../tools/conflict_io_selective.patch);
não foi aplicado à implementação de produção.

## Método

Linux/amd64, Go 1.26.0, Ryzen 5 5600GT, ext4 em `/dev/sda2`, GC padrão,
binários sem cgo. Corpus e scratch em disco dentro de `.measure/conflict-io`.
Nenhum corpus do usuário foi fornecido ou acessado. Não houve controle de
cache frio nem alteração global de caches; os processos foram sequenciais,
com ordem alternada AB/BA/AB nas três repetições principais.

O [medidor](../tools/buildmeasure/main.go) contabiliza bytes efetivamente
retornados por `ReadAt` e `Write` do port de scratch, inclusive leituras
parciais. Esses são **bytes lógicos**, não tráfego físico do dispositivo.
Leituras de páginas mmap não entram no contador Read. O pico de scratch
contabiliza tamanhos lógicos dos arquivos vivos, sem input, snapshot ou
blocos/metadados do filesystem. GNU time mede RSS do processo inteiro,
incluindo publicação. Tempos de build e publicação são separados.

O sink calcula SHA-256 de todos os diagnósticos JSON em ordem; seu custo
está no tempo de build de ambas as variantes. Fora desse tempo, a campanha
confere SHA-256 de snapshots, reports sem durações, contagem e digest de
diagnósticos. Um desvio ou scratch residual encerra a campanha com erro.

Índices `group` e `score`, orçamento principal de 16 MiB. Todos os perfis têm
400 mil nodes. O [gerador/campanha](../tools/conflict_io_measure.py) produz:

| Perfil | Registros | Edges | Contribuições de propriedade | Precisam de sequência em first/last |
|---|---:|---:|---:|---:|
| Sets/edges | 2,4 milhões | 2 milhões | 2,8 milhões | 0% |
| Misto | 2,8 milhões | 2 milhões | 3,2 milhões | 25% |
| Somente single de nodes | 1,2 milhão | 0 | 2,4 milhões | 100% |

Sets/edges reproduz a topologia do gerador existente: ciclos, hubs, paralelas,
loops, labels e propriedades tipadas. Misto acrescenta uma contribuição
conflitante de `rank:Int(single)` por node. Single usa três passes A→B→A em
duas propriedades por node: testa que repetir o valor original participa da
precedência e que o vencedor atravessa runs. As fontes usam a mesma chave
`data.csv` nas duas variantes.

## Resultados com 16 MiB

Três repetições por variante/perfil, 18 builds. Medianas:

| Perfil | I/O scratch total, atual → variante | Redução | Pico scratch, atual → variante | Build, atual → variante |
|---|---:|---:|---:|---:|
| Sets/edges | 14,236 → 13,991 GiB | 1,72% | 1.179,00 → 1.163,45 MiB | 35,91 → 36,13 s |
| Misto | 15,476 → 15,259 GiB | 1,40% | 1.179,00 → 1.163,45 MiB | 38,49 → 39,54 s |
| Somente single | 6,005 → 6,005 GiB | 0% | 940,71 → 940,71 MiB | 13,90 → 14,14 s |

I/O total é Read + Written. Os volumes foram exatamente iguais nas três
repetições de cada variante. Sets/edges reduziu leitura em 1,87% e escrita
em 1,58%; misto, 1,51% e 1,28%. O pico de scratch caiu 15,55 MiB nos dois
perfis com contribuições dispensáveis. RSS mediano nesses perfis ficou
praticamente igual, perto de 406 MiB, com diferença inferior a 0,2%.

Os tempos individuais de sets/edges foram 35,39–36,34 s no atual e
36,00–37,56 s na variante. No misto, 38,46–39,71 s e 38,30–40,12 s.
Os deltas pareados da variante foram +2,11%, +3,36%, +0,26% em sets/edges;
+1,05%, −0,42%, +2,71% no misto. Single foi +3,55%, +1,68%, +0,07%.
Essas poucas repetições não demonstram uma regressão estatisticamente
estabelecida; também não mostram aceleração consistente. O controle single
move os mesmos bytes, mas apresenta variação de tempo da mesma ordem.

## Conferência com 1 MiB e avaliação

Um par adicional por perfil, quatro builds exploratórios:

| Perfil | I/O scratch total, atual → variante | Redução | Build, atual → variante |
|---|---:|---:|---:|
| Sets/edges | 21,686 → 21,266 GiB | 1,94% | 45,77 → 49,76 s |
| Misto | 23,553 → 23,185 GiB | 1,56% | 49,82 → 49,46 s |

Mais passes aumentaram a economia absoluta, mas não tornaram a diferença
grande no pipeline inteiro. Pico de scratch novamente caiu cerca de 1,32%;
RSS permaneceu praticamente igual. Esse par não qualifica diferenças de
latência, especialmente o +8,71% observado em sets/edges.

**Decisão aprovada pelo usuário: manter a implementação atual no PR #6.** Há evidência
reprodutível de economia de bytes, entre 1,40% e 1,94% nos perfis que permitem
omitir sequência. Não há evidência de aceleração consistente nem de redução
relevante de RSS. A sequência é descartada na consolidação; os passes posteriores
de dicionários, colunas e CSR não carregam esses bytes. A economia de scratch
é modesta frente à mudança no layout temporário e na sua leitura.
Uma decisão futura pode usar um corpus real com
limitação de scratch I/O ou espaço, ou propor a otimização em PR separado.
O patch experimental é material de reprodução e não participa dos builds
de produção; essa otimização não é uma pendência desta entrega.

Todos os 22 builds grandes produziram snapshots, reports sem tempos e
diagnósticos idênticos entre variantes/orçamentos para cada perfil; nenhum
workspace residual. Houve 400 mil warnings de conflito no misto e 800 mil
no controle single. O piloto de 10 mil nodes também passou. A variante
passou nos testes reais de ingestão e snapshot, incluindo os três modos,
conflitos de cardinalidade, grupos entre runs e falhas de scratch.
Race de ingestão/snapshot passou no atual e na variante; race e vet do
medidor, build sem cgo, py_compile, reducer e checker documental passaram.
O checker documental não substitui os testes da engine. A produção não foi
alterada e a suíte completa anteriormente executada do PR não foi repetida.

Dados brutos: [16 MiB](benchmarks/node_conflict_io_16m.jsonl),
[1 MiB](benchmarks/node_conflict_io_1m.jsonl),
[corpora e binários](benchmarks/node_conflict_io_manifest.json) e
[resumo reduzido](benchmarks/node_conflict_io_summary.json).

## Reproduzir

Use filesystem em disco com alguns GiB livres. Compilar é separado das
medições. A cópia experimental contém a mesma base de produção, mais o
medidor atualizado e o patch de layout temporário:

```sh
mkdir -p .measure/conflict-io/replay/selective
git archive d524c9c | tar -x -C .measure/conflict-io/replay/selective
cp tools/buildmeasure/main.go tools/buildmeasure/scratch.go \
  .measure/conflict-io/replay/selective/tools/buildmeasure/
patch -d .measure/conflict-io/replay/selective -p1 < tools/conflict_io_selective.patch
GOPROXY=off GOTOOLCHAIN=local CGO_ENABLED=0 go build -buildvcs=false \
  -o .measure/conflict-io/replay/current ./tools/buildmeasure
TASK_MEASURE_ROOT="$PWD/.measure/conflict-io/replay"
(
  cd "$TASK_MEASURE_ROOT/selective"
  GOPROXY=off GOTOOLCHAIN=local CGO_ENABLED=0 go build -buildvcs=false \
    -o "$TASK_MEASURE_ROOT/candidate" ./tools/buildmeasure
)
python3 tools/conflict_io_measure.py \
  --baseline .measure/conflict-io/replay/current \
  --candidate .measure/conflict-io/replay/candidate \
  --root .measure/conflict-io/replay/large --nodes 400000 --repeats 3
python3 tools/conflict_io_measure.py \
  --baseline .measure/conflict-io/replay/current \
  --candidate .measure/conflict-io/replay/candidate \
  --root .measure/conflict-io/replay/sensitivity --nodes 400000 --repeats 1 \
  --budgets 1048576 --profiles sets-edges mixed
python3 tools/conflict_io_summary.py \
  .measure/conflict-io/replay/large/samples.jsonl \
  .measure/conflict-io/replay/sensitivity/samples.jsonl
```

O reducer verifica novamente igualdade dos resultados antes de calcular
medianas, intervalos e diferenças pareadas. Um par com 1 MiB serve apenas
como verificação exploratória do volume de I/O sob mais passes de merge.

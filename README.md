# GopherGraph

Motor Go para carregar um multigrafo direcionado com propriedades e consultar snapshots imutáveis. Inclui biblioteca reutilizável, ingestão resiliente de Neptune Gremlin CSV, CSR nos dois sentidos, índices compactos, mmap e CLI local.

## Preparar e compilar

Plataforma do produto: **Linux/amd64**, com **Go 1.26** instalado globalmente. Testes também usam Python 3; race exige GCC/libc. GNU time serve às medições e Graphviz é opcional para conferir DOT.

Em Ubuntu, as ferramentas podem ser instaladas globalmente com:

```sh
sudo apt-get install golang-go build-essential python3 time graphviz
```

A única dependência Go é `golang.org/x/sys/unix`, isolada no adapter de mmap. Prepare o cache pelo gerenciador normal de módulos antes de construir/testar:

```sh
GOTOOLCHAIN=local go mod download
CGO_ENABLED=0 go build -o bin/gophergraph ./cmd/gophergraph
./bin/gophergraph --help
```

O binário de produção não usa cgo. Os testes não instalam dependências nem acessam serviços remotos.

## Carregar e consultar

Nodes e edges vêm de **dois diretórios distintos**. Cada catálogo considera apenas entradas imediatas, sem filtro por extensão; symlinks e diretórios são rejeitados individualmente. Todos os nodes são consolidados antes de ler edges.

Exemplo reproduzível com as fixtures incluídas:

```sh
./bin/gophergraph build \
  --nodes fixtures/01_topology/nodes \
  --edges fixtures/01_topology/edges \
  --output bin/graph.snapshot \
  --index-property sigla

./bin/gophergraph territory --snapshot bin/graph.snapshot --node A
./bin/gophergraph anti-territory --snapshot bin/graph.snapshot --node F
./bin/gophergraph between --snapshot bin/graph.snapshot --from A --to F

./bin/gophergraph territory --snapshot bin/graph.snapshot --node A \
  --edge-label CALLS --format dot --output bin/territory.dot
```

A primeira consulta imprime:

```csv
id
B
C
D
F
```

- CSV de IDs omite a origem por padrão; `--include-origin` a inclui. DOT sempre inclui a origem e preserva paralelas e loops.
- Um ID vazio é válido: passe `--node=""`. IDs com vírgulas, aspas e quebras de linha recebem escaping CSV.
- Sem `--edge-label`, todas as relações são permitidas. Um label inexistente produz filtro vazio e alcance reflexivo.
- `between(A,B)` retorna a interseção do alcance forward de A com o alcance reverse de B, com todas as edges permitidas entre os membros. Em ciclos, essa região pode incluir passeios que não são caminhos simples.
- `--index-property` pode repetir. Igualdade tipada produz o mesmo resultado com índice ou scan, preservando distinções entre tags, NaN canônico e zeros assinados.

### Cargas parciais

```sh
./bin/gophergraph build \
  --nodes fixtures/02_resilient/nodes \
  --edges fixtures/02_resilient/edges \
  --output bin/partial.snapshot
./bin/gophergraph territory --snapshot bin/partial.snapshot --node A
```

Registros inválidos, headers errados, fontes com falha e endpoints ausentes geram diagnósticos JSON em stderr. A carga continua sobre os dados aceitos e o resumo informa `PARTIAL`. Propriedades conflitantes são removidas; um EdgeID estruturalmente conflitante é quarentenado. Aspas abertas até EOF encerram apenas a fonte, preservando seu prefixo completo.

Queries mantêm os dados limpos em stdout e avisam em stderr quando o snapshot veio de carga parcial. Cancelamento, falha do catálogo raiz ou do diagnóstico impedem publicação. O limite padrão por registro é 64 MiB, ajustável com `--max-record-bytes` (mínimo 1024).

| Exit code | Significado |
|---:|---|
| 0 | Sucesso operacional, inclusive carga parcial publicada |
| 1 | Falha operacional, cancelamento ou durabilidade não confirmada |
| 2 | Argumentos ou caminhos de configuração inválidos |
| 3 | ID requerido não encontrado |

Output de build deve ficar fora dos catálogos. Output de query não pode ser o próprio snapshot ou um alias dele.

## Build com memória limitada

A CLI usa ordenação externa e arquivos temporários por padrão. O orçamento de
ordenação é 64 MiB; o diretório temporário padrão é o diretório do snapshot.
Para controlar recursos:

```sh
./bin/gophergraph build \
  --nodes fixtures/01_topology/nodes --edges fixtures/01_topology/edges \
  --output bin/graph.snapshot --memory-budget 16777216 --temp-dir bin \
  --max-record-bytes 1048576
```

`--memory-budget` limita buffers de ordenação, com mínimo de 1 MiB. O heap também
inclui o registro/header atual, buffers fixos, entradas dos catálogos e opções.
RSS inclui páginas de arquivos mmap recuperáveis pelo sistema operacional.
Use um filesystem em disco para scratch; tmpfs usa RAM. Reserve espaço para
passes intermediários e a publicação, conforme as [medições](docs/BOUNDED_INGEST.md).
Falta de espaço, erro de scratch ou cancelamento falham o build sem publicar
um prefixo. Temporários são removidos nos retornos normais e de erro.

## Usar como biblioteca

`graph` oferece metadados, valores tipados, lookup, adjacências e conjuntos com identidade. `query` oferece `Reachable`, `FromNodes`, `Territory`, `AntiTerritory` e `Between`. `ingest.Build`, `snapshot.Open` e `snapshot.Write` recebem ports; os adapters concretos ficam na composição do programa.

Uma consulta própria pode usar adjacências diretamente ou combinar resultados. O [exemplo shared_targets](examples/shared_targets/query.go) usa somente API pública, sem registry:

```sh
go run ./examples/shared_targets/cmd bin/graph.snapshot A B
```

Ele imprime `"B"`, `"D"` e `"F"`, um ID por linha JSON. O teste E2E também compila essa consulta em um módulo consumidor separado, com acesso apenas à API pública.

### Ownership do snapshot

O dono deve chamar `Graph.Close()` depois que todas as queries e iteradores terminarem. Close sequencial é idempotente; não há suporte para Close concorrente com leitores. Strings públicas são cópias seguras, e slices do armazenamento não são expostos para escrita.

A publicação usa tempfile, validação, sync, rename e sync do diretório. Leitores antigos continuam usando o inode anterior. `PublishedUncertain` informa que o novo nome já está visível, mas a durabilidade não foi confirmada.

**Não reescreva nem trunque um inode que tenha leitores mmap ativos.** `MAP_PRIVATE` não protege contra alteração externa; truncamento pode causar SIGBUS. Para substituir snapshots, use o fluxo de publicação da biblioteca/CLI.

## Verificação

Com o cache de módulos preparado:

```sh
GOPROXY=off GOTOOLCHAIN=local go test -count=1 ./...
go vet ./...
CGO_ENABLED=0 go build -o bin/gophergraph ./cmd/gophergraph
CGO_ENABLED=1 go test -race ./...
go test ./ingest/adapters/neptune -run '^$' -fuzz '^FuzzNeptuneCSV$' -fuzztime=10000x
go test ./snapshot -run '^$' -fuzz '^FuzzSnapshotDecode$' -fuzztime=10000x
go test -run '^$' -bench . -benchmem ./...
python3 tools/check_package.py
```

A suíte inclui os 12 modelos de fixtures, oráculo de closure, seis goldens binários independentes, reader Python, corrupção, falhas de ports/publicação, concorrência, recursos, CLI em subprocessos e consumidor externo. O checker Python confere referências e documentos; os testes Go verificam a engine.

[Medições locais](docs/BENCHMARKS.md) separam build, validação, consultas, exportação, heap, mapping e RSS. A [qualificação da ingestão limitada](docs/BOUNDED_INGEST.md) cobre até 400 mil nodes / 2 milhões de edges sintéticos, sem prometer SLA ou desempenho sobre corpus não fornecido.

## Contratos e continuidade

[PROGRESS.md](PROGRESS.md) concentra o estado e as evidências dos checkpoints. [SCOPE](SCOPE.md), [ARCHITECTURE](docs/ARCHITECTURE.md), [API](docs/API.md), [INGEST](docs/INGEST.md), [SNAPSHOT](docs/SNAPSHOT.md), [CLI](docs/CLI.md) e [TESTING](docs/TESTING.md) detalham os contratos. Para retomar desenvolvimento, use [START_HERE](START_HERE.md).

HTTP, S3, mutação online, DSL, planner e execução distribuída estão fora deste escopo. Não foi feito acesso a Neptune, cloud ou dados corporativos.

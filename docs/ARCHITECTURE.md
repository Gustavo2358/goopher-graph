# Arquitetura por capacidades

## Desenho

GopherGraph tem um núcleo de grafo imutável, uma capacidade de ingestão, uma de persistência e bibliotecas de consulta/exportação. O driver compõe as peças. Ports and adapters são uma regra de dependência, não uma árvore horizontal nem um framework.

```text
cmd/gophergraph (argv, sinais, composição, códigos de saída)
     |                 |                    |
     v                 v                    v
ingest.Build      snapshot.Open         query.* + dot.Write / graphjson.Write / ggpb.Write
     |                 |                    |
     +-----------------+--------------------+
                       v
                  graph.Graph
             IDs + colunas + CSR
```

Adapters implementam interfaces definidas pela capacidade consumidora:

```text
ingest/ports <- ingest/adapters/filesystem
             <- ingest/adapters/neptune
             <- ingest/adapters/stderr

snapshot/ports <- snapshot/adapters/file
               <- snapshot/adapters/mmap
```

A dependência é adapter -> port. A capacidade não instancia o adapter concreto. A CLI instancia catálogos filesystem, decoder Neptune, diagnóstico e destino local, chama ingestão, persistência ou consulta e traduz resultados para output. Nunca contém merge, BFS, política de rejeição ou conhecimento do layout.

## Árvore alvo

A árvore é um mapa de responsabilidades. Criar arquivos conforme a fatia; não gerar diretórios vazios e stubs apenas para se parecer com o desenho. Testes `_test.go` ficam junto à capacidade.

```text
gophergraph/
├── go.mod                         # criado em B00
├── go.sum                         # quando a dependência de mmap entrar
├── cmd/gophergraph/
│   ├── main.go
│   ├── build.go
│   └── query.go
├── graph/
│   ├── graph.go                   # objeto imutável, identidade e lifecycle
│   ├── ids.go
│   ├── value.go                   # tipos e igualdade canônica
│   ├── adjacency.go               # iterador por valor, sem alocação por edge
│   ├── set.go                     # NodeSet/EdgeSet e bitsets
│   ├── index.go
│   └── graph_test.go
├── ingest/
│   ├── build.go                   # nodes -> barreira -> edges
│   ├── model.go                   # staging e contribuições
│   ├── merge.go
│   ├── canonicalize.go
│   ├── csr.go
│   ├── index.go
│   ├── report.go
│   ├── ports/
│   │   ├── catalog.go
│   │   ├── records.go
│   │   └── diagnostics.go
│   └── adapters/
│       ├── filesystem/catalog.go
│       ├── neptune/
│       │   ├── reader.go
│       │   ├── framing.go         # presença/aspas/limites; não DSL
│       │   ├── header.go
│       │   └── values.go
│       └── stderr/sink.go
├── snapshot/
│   ├── format.go
│   ├── reader.go
│   ├── writer.go
│   ├── validate.go
│   ├── ports/
│   │   ├── source.go
│   │   └── sink.go
│   └── adapters/
│       ├── mmap/source_linux.go
│       └── file/sink.go
├── query/
│   ├── reachable.go
│   ├── subgraph.go
│   ├── territory.go
│   ├── anti_territory.go
│   └── between.go
├── graphjson/writer.go            # JSON tipado por streaming
├── ggpb/                         # batches Protobuf, framing e reader incremental
│   └── pb/result.proto           # contrato lógico público, independente do snapshot
├── dot/writer.go
├── internal/graphdata/             # somente colunas/layout lógico compartilhado
│   ├── columns.go
│   └── validate.go
├── tests/e2e/
├── examples/shared_targets/        # programa Go externo à lógica do core
├── fixtures/
└── docs/ + spec/ + tools/
```

`internal/graphdata` é uma pequena implementação privada da **mesma capacidade de grafo**, usada por graph, ingest e snapshot para trocar colunas sem ciclos de import. Não é uma nova camada global. Não guarda I/O, política de carga, strings CSV, AWS nem UI. A forma exata dos seus helpers é detalhe privado; não expô-la ao consumidor externo.

## Regras de imports

| Área | Pode conhecer | Não pode conhecer |
|---|---|---|
| graph, internal/graphdata | stdlib de dados/erros/bytes | adapters, CSV, mmap, paths, HTTP |
| query | graph, context e estruturas próprias | ingestão, persistência, CLI |
| ingest | graph/graphdata e seus ports | filesystem concreto, Neptune concreto, SDK |
| ingest/ports | tipos de contribuição e tipos simples | implementação do builder ou adapters |
| snapshot | graph/graphdata, seus ports, binário LE | mmap/file concretos, CLI |
| dot, graphjson, ggpb | graph, query, io.Writer | Graphviz, os.Create, stdout global, HTTP |
| adapters | port e APIs concretas necessárias | regras de negócio de consultas |
| cmd | capacidades públicas e adapters locais | layout privado, algoritmos |

Tipos de records normalizados usados por ingest e pelo decoder ficam em `ingest/ports`; assim um adapter não cria um ciclo importando o orquestrador. Tipos de owner/bytes/publicação ficam em `snapshot/ports`. Evitar um pacote `common` que passe a conter tudo.

A checagem arquitetural de teste inspeciona imports próprios, não todas as dependências transitivas internas da stdlib. Uma biblioteca padrão pode usar `os` internamente sem que o nosso domínio conheça paths.

## Representação e hot path

Graph expõe métodos e iteradores; internamente usa colunas de IDs/offsets, não objetos ligados por ponteiros. As colunas podem ser arrays heap nas primeiras fatias e views de bytes LE depois. O acesso escalar fica em helpers concretos pequenos, sem uma interface por acesso a vizinho. Um branch de backend ou uma leitura LE é escolha de implementação mensurável, não uma razão para espalhar `unsafe`.

O formato de arquivo é responsabilidade de snapshot. `internal/graphdata` só conhece colunas, contagens, tipos e invariantes lógicas. Não recebe offsets do diretório on-disk como parte da API pública.

A CLI passa `Options.Scratch` para ingestão externa: contribuições atômicas,
ordenação externa, consolidação em streams e colunas temporárias mmap. O port
`ingest/ports.Scratch` cria um workspace privado por build; o adapter filesystem
usa o adapter mmap já existente. O núcleo não importa filesystem ou mmap.
Sem Scratch, `ingest.Build` conserva o caminho heap para consumidores existentes
e testes de equivalência. Ver [desenho e limites](BOUNDED_INGEST.md).

A representação de consulta não herda mapas de staging. Canonicalização encerra staging e fixa IDs. Índices são postings; scratch de consultas são bitsets/fila. Dados do mapping não são expostos como `[]byte` ou slices graváveis ao consumidor.

## Reuso e evolução

Uma consulta adicional importa `graph` e `query`, implementa funções normais e recebe seu próprio driver. Não precisa alterar `cmd/gophergraph`, registrar plugins ou acessar `internal/`.

Futuro S3 implementa catálogo/abertura de streams; futuro HTTP chama as mesmas funções. Não mapear objeto S3 diretamente: o adapter materializa um snapshot local/estável antes da abertura. Nenhum desses adapters futuros é implementado agora.

Um único processo pode executar consultas simultâneas sobre o mesmo Graph aberto, com scratch por chamada. Não há goroutines de background no core, nem necessidade de pool ou locks por edge. O driver garante quiescência antes do Close; hot swap de snapshots é outro escopo.

## Queries WebAssembly

`cmd/gophergraph -> wasmquery -> graph/query`; o resultado segue para
`graphjson.Write` (padrão) ou `ggpb.Write` quando solicitado. `wasmquery` recebe Graph, bytes WASM, argumentos e contexto.
Não importa filesystem, snapshot ou transporte. Wazero fica isolado nessa
capacidade. `wasmquery/sdk` depende apenas de stdlib e da definição privada da
ABI, sem importar engine/runtime. Os exemplos externos usam somente o SDK.

Uma tabela privada por execução associa tokens a NodeSets, EdgeSets e Subgraphs.
As operações usam APIs públicas do grafo e mantêm trabalho intensivo no host.
Módulos compilados imutáveis são cacheados por conteúdo no runtime; instâncias
WASI, conjuntos e contexto de execução são novos por chamada. A Host API não
expõe memória interna nem permite mutar o Graph. [Contrato e lifecycle](WASM.md).

## Resultados GGPB

`ggpb → graph/query`, com mensagens geradas em `ggpb/pb`. Protobuf fica isolado
nessa capacidade. `ggpb.Emit` entrega batches lógicos sincrônicos; `ggpb.Write`
adiciona framing para io.Writer. Reader/JSON decodificam sem Graph/snapshot.
Dictionaries são locais ao batch e partes permitem dividir uma entidade com
muitas propriedades. A única extensão do core foi IterateNodeLabels, sem cópia, para limitar memória
mesmo em nodes com muitos labels. NodeLabels e o layout permanecem intactos.
[Contrato, memória, versionamento e futuro gRPC](GGPB.md).

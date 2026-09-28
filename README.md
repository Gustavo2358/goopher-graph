# GopherGraph

**Um motor de grafos pequeno, escrito em Go. CSV entra; consultas atravessam um snapshot imutável.**

GopherGraph reúne ingestão local compatível com o **Gremlin CSV do bulk loader do Neptune**, grafo direcionado com propriedades, CSR forward/reverse, persistência binária e leitura por mmap. Consultas são funções e programas Go que usam a API, não expressões em uma linguagem inventada.

O nome combina *Gopher* com *Graph*. É um nome de trabalho do projeto; não presume registro de marca, domínio disponível ou módulo publicado.

## Começar

Abra esta pasta como raiz da sessão do agente e use o prompt de [START_HERE.md](START_HERE.md). [AGENTS.md](AGENTS.md) rege a execução; [SCOPE.md](SCOPE.md) define o produto; [BACKLOG.md](BACKLOG.md) vai do primeiro teste ao fechamento. **Todas as fatias começam pendentes em [PROGRESS.md](PROGRESS.md).**

Este pacote contém especificação e material de referência, **não uma engine implementada**. Não depende de documentos anteriores, de um projeto C ou da conversa que o originou. Não há migração de produto nem séries de versões documentais.

## Produto que será construído

```text
nodes/ + edges/ (declarados separadamente)
          |
  ingestão resiliente: todos os nodes, depois edges
          |
   canonicalização + CSR + índices
          |
    graph.snapshot (imutável)
          |
   mmap local -> graph.Graph
          |
   território / antiterritório / between / programas Go
          |
           CSV de IDs ou DOT
```

A estrutura é por capacidade: `graph`, `ingest`, `snapshot`, `query`, `dot`. Interfaces pequenas e adapters ficam na capacidade que possui a fronteira. CLI é apenas um driver. Não existe uma divisão horizontal global em application/domain/infrastructure.

## Mapa de leitura

| Preciso entender | Documento |
|---|---|
| Limites e motivo das escolhas | [SCOPE](SCOPE.md), [decisões](docs/DECISIONS.md) |
| Packages, imports, Go idiomático | [arquitetura](docs/ARCHITECTURE.md), [práticas Go](docs/GO_ENGINEERING.md) |
| Identidade, tipos, CSR e índices | [modelo](docs/DATA_MODEL.md) |
| Nodes/edges, CSV, rejeições e relatórios | [ingestão](docs/INGEST.md), [ports](docs/PORTS.md) |
| Arquivo byte a byte, validação, mmap e publicação | [snapshot](docs/SNAPSHOT.md) |
| API, ownership e extensão por programas | [API](docs/API.md), [consultas](docs/QUERIES.md) |
| Comandos que existirão | [CLI](docs/CLI.md) |
| Gates e evidências necessárias | [testes](docs/TESTING.md), [desempenho](docs/PERFORMANCE.md), [fechamento](docs/CLOSEOUT.md) |
| Fontes externas e limites da compatibilidade | [referências](docs/REFERENCES.md) |
| Layout e exemplos verificáveis | [spec](spec/README.md), [fixtures](fixtures/README.md) |

## Desenvolvimento lean

Um módulo Go, stdlib como padrão e `golang.org/x/sys/unix` restrito ao adapter de mmap/Linux. Nada de cgo, framework web, SDK AWS, parser de queries ou gerador de bindings no produto inicial. `go.mod`/`go.sum` são arquivos normais da ferramenta, não um ritual de aprovação. A versão de Go/dependência usada é a disponível e aprovada no ambiente, sem exigir um patch específico nesta spec.

A abertura segura e o ciclo de vida do mapping são requisitos; `mmap` não significa acesso instantâneo nem ausência de consumo de memória. Desempenho é medido, não presumido pela linguagem.

O verificador opcional `python3 tools/check_package.py` confere documentos, modelos e referências. Ele não implementa ingestão nem substitui `go test`. [PACKAGE_CHECK.md](PACKAGE_CHECK.md) distingue exatamente o que foi verificado na entrega.

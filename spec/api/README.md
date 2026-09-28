# Assinaturas Go de referência

Os arquivos `.go.txt` usam sintaxe Go, com corpos omitidos. São **documentação da superfície pública**, não packages implementados, mocks ou stubs a copiar para o produto. Structs privadas aparecem vazias por notação: os campos reais são decisão interna.

| Arquivo | Contrato |
|---|---|
| [graph.go.txt](graph.go.txt) | Identidade, valores, adjacência, propriedades, sets e lifecycle |
| [query.go.txt](query.go.txt) | Alcance, subgrafo e consultas compostas |
| [ingest_ports.go.txt](ingest_ports.go.txt) | Catálogos, records normalizados, decoder e diagnósticos |
| [ingest.go.txt](ingest.go.txt) | Build, options e relatório |
| [snapshot_ports.go.txt](snapshot_ports.go.txt) | Backing, source e publicação |
| [snapshot.go.txt](snapshot.go.txt) | Codec desacoplado do transporte |
| [dot.go.txt](dot.go.txt) | Saída via io.Writer |

`gophergraph/...` é o caminho local do módulo a criar em B00. Não aponta para um serviço externo. O contrato de erros/ownership em docs/API e docs/PORTS complementa as assinaturas. Helpers privados, distribuição de arquivos e construção entre capacidades podem ser decididos pelo agente, sem expor `internal/graphdata` ao consumidor de consultas.

`gofmt` consegue verificar a sintaxe desses snippets; isso **não compila nem resolve imports da engine ausente**. Em B09, o exemplo deve compilar e rodar contra a API real. Os testes desse pacote documental não satisfazem esse gate.

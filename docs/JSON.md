# JSON de subgrafos

`territory`, `anti-territory` e `between` aceitam `--format json`. A saída é um
objeto JSON UTF-8 seguido de newline. Inclui todo o `query.Subgraph`, inclusive
a origem quando ela pertence ao resultado. `--include-origin` afeta somente
`ids`. `--output` escolhe arquivo; sem ele, a saída vai a stdout.

Exemplo de formato (indentado apenas para leitura):

```json
{
  "query": {"name": "territory", "node": "A", "edgeLabels": null},
  "directed": true,
  "partialSnapshot": false,
  "counts": {"nodes": 1, "edges": 1},
  "nodes": [
    {"id": "A", "labels": ["ENTRY", "PROGRAM"], "properties": [
      {"key": "tags", "type": "String", "value": "one"},
      {"key": "tags", "type": "String", "value": "two"}
    ]}
  ],
  "edges": [
    {"id": "loop", "source": "A", "target": "A", "label": "CALLS", "properties": [
      {"key": "sequence", "type": "Long", "value": "9223372036854775807"}
    ]}
  ]
}
```

## Estrutura e metadata

- `nodes` e `edges` são arrays, inclusive quando vazios (`[]`). Cada membro
  aparece uma vez. Edges paralelas conservam IDs, labels e propriedades próprios.
- Todos os IDs são externos, inclusive `source`, `target` e parâmetros da query.
  `source` → `target` é a direção original, também em `anti-territory`.
- Nodes têm `labels[]`; edges têm um `label`. `properties[]` contém uma entrada
  por valor tipado. Chaves repetidas preservam os valores de propriedades set.
  Propriedades ausentes não têm entradas; string vazia continua `""`. O snapshot
  não conserva anotações de cardinalidade do CSV; a saída descreve seus valores.
- `query.name` é `territory`, `anti-territory` ou `between` na CLI. Os dois
  primeiros incluem `node`; `between` inclui `from` e `to`. ID vazio é válido.
- `query.edgeLabels`: `null` significa sem restrição; array significa união dos
  labels solicitados. É ordenado e sem duplicatas, conservando labels
  desconhecidos. Na biblioteca, `[]` descreve um filtro que não permite edges.
- `counts.nodes` e `counts.edges` contam os membros exportados.
  `partialSnapshot` vem do snapshot inteiro: `true` indica perdas na ingestão,
  mesmo que o subgrafo consultado pareça completo. O aviso CLI em stderr continua.

## Valores tipados

| `type` | Representação de `value` |
|---|---|
| `Bool` | boolean JSON |
| `Byte`, `Short`, `Int` | número inteiro JSON |
| `Long` | **sempre string decimal**, incluindo valores pequenos; converter para int64/BigInt |
| `Float`, `Double` | número JSON quando finito; strings `"NaN"`, `"+Inf"`, `"-Inf"` nos demais casos |
| `String`, `Date`, `Datetime` | string JSON, preservando o texto armazenado |

Floats finitos usam a menor representação decimal que recupera o valor na
precisão do tipo (`Float`: 32 bits; `Double`: 64 bits). Zero negativo é `-0`;
leitores devem usar parsing de ponto flutuante para preservar o sinal. NaN já
é canônico no grafo. Não converter `Long` para um número IEEE-754 de 64 bits:
isso perde precisão fora de ±(2^53−1).

Nodes e edges saem em ordem crescente de ID externo; labels e propriedades
seguem a ordem canônica do grafo. A mesma consulta e grafo produzem os mesmos
bytes, inclusive quando a ordem/repetição dos filtros varia. Strings usam
escaping de `encoding/json`; não há timestamps nem IDs internos no documento.

## Biblioteca e streaming

```go
err := graphjson.Write(ctx, output, g, sub, graphjson.Query{
    Name: "territory", Node: &origin, EdgeLabels: requestedLabels,
})
```

`g` é `*graph.Graph`, `sub` é `*query.Subgraph` e `output` é `io.Writer`.
O chamador fornece a metadata da consulta executada; o writer não recalcula
consultas. Ponteiros nil omitem os parâmetros opcionais; `&origin` conserva
um ID vazio. Queries Go próprias também podem fornecer seu nome.

O writer usa os iteradores públicos e um buffer fixo de escrita. Memória
auxiliar depende do maior escalar, dos labels do node atual e dos parâmetros
da query, sem acumular entidades ou propriedades do resultado inteiro.
O destino também deve consumir os bytes aos poucos para manter esse limite;
um `bytes.Buffer` escolhido pelo chamador acumula a saída.

Mantenha o grafo aberto e subgrafo/parâmetros sem mutação durante a escrita.
O writer verifica contexto e erros, inclusive short writes e flush. Em caso de
erro, descarte o prefixo incompleto. Ele não fecha o destino; seu fechamento
pertence ao chamador.

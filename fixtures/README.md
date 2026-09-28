# Fixtures de aceitação

Cada pasta contém `nodes/`, `edges/` e `expected.json`. São entradas do produto a implementar; **não resultados de uma engine já executada**. Os diretórios explicitamente separados fazem parte do contrato.

| Pasta | Foco |
|---|---|
| 01_topology | Direção, diamante, ciclo, loop, paralelas, labels e índices |
| 02_resilient | Registros ruins entre válidos e headers de papel errado |
| 03_typed_values | Todas as tags, empty/blank, limites, Unicode, multiline e escapes |
| 04_conflicts | Merge, conflitos e tombstones sem dependência de ordem |
| 05_defaults_empty_id | Defaults, arquivo sem extensão CSV e IDs externos vazios |
| 06_unrecoverable_csv | Preservar prefixo e continuar na próxima fonte |
| 07_empty | Snapshot vazio completo |
| 08_all_rejected | Snapshot vazio parcial, sem abortar por bad data |
| 09_invalid_utf8 | Byte inválido em registro com delimitação recuperável |
| 10_cardinality | Conflito single/set e header single[] rejeitado |
| 11_csv_presence_crlf | Ausência/quoted vazio, CRLF dentro de string e espaços preservados |
| 12_numeric_dialect | INF/hex recusados, zeros assinados e NaN canônico |

## Esquema de expected.json

`graph` é o modelo final, já consolidado; `index_properties` é o conjunto de opções do builder. `load.completeness` e counts são obrigatórios. `required_diagnostics` relaciona código e número esperado de eventos semânticos para a fixture; não conta mensagens informativas de progresso. `report_exact` fixa contadores especialmente importantes, sem exigir campos ainda não relevantes ao caso.

`queries` registra kind, origem/fim, filtro de labels (`null` = todas; `[]` = nenhuma), nodes do conjunto reflexivo, EdgeIDs do subgrafo e IDs padrão da listagem CLI. A ordem de comparação é canônica, não a ordem textual dos registros. Valores especiais de Float/Double usam strings `NaN`, `Infinity`, `-Infinity` no JSON de referência; strings de propriedades reais continuam com tag String.

`lexically_invalid_sources` lista somente fontes intencionalmente impossíveis de ler integralmente como CSV UTF-8. Não pular essas fontes nos testes de engine: elas existem para testar rejeição/recuperação. O verificador documental apenas evita tratá-las como fixtures lexicais positivas.

## Como usar

B01/B02 podem montar o `graph` esperado diretamente em arrays de teste. B03 testa o decoder. B04/B05 comparam a carga desses inputs com o modelo esperado. B07–B09 comparam snapshots, APIs e CLIs. Não derivar o esperado da implementação Go em teste.

Casos de falha de sistema, limite grande e grafos aleatórios são gerados durante os testes conforme TESTING.md; não empacotamos arquivos enormes ou binários de produto. O diretório `snapshot_reference/` contém goldens pequenos do formato e as associações aos modelos.

Códigos estáveis exigidos aqui: HEADER_INVALID, VALUE_INVALID, REQUIRED_FIELD_MISSING, ENDPOINT_NOT_FOUND, BOOL_COERCED, PROPERTY_CONFLICT, PROPERTY_CARDINALITY_CONFLICT, EDGE_ID_CONFLICT, CSV_SOURCE_UNRECOVERABLE, UTF8_INVALID e EMPTY_GRAPH. O produto pode acrescentar outros códigos para falhas distintas, sem substituir esses por análise do texto de mensagens.

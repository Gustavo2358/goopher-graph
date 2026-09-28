# Modelo de dados e representação em memória

## Entidades e identidade

Um **node** tem ID externo UTF-8, um conjunto não vazio de labels e zero ou mais propriedades. Uma **edge** tem ID externo UTF-8, origem, destino, exatamente um label e propriedades escalares. A direção é sempre `source -> target`. IDs externos de nodes e edges ocupam espaços distintos; uma string pode identificar um node e uma edge sem colisão. Arestas paralelas e self-loops são válidos.

A contribuição sem label recebe `vertex` ou `edge` no adapter de entrada. Todos os nomes de domínio são dados. A propriedade `sigla` não tem tratamento especial; apenas pode ser escolhida para indexação.

## IDs e limites

`NodeID`, `EdgeID` e `StringID` são `uint32`. `math.MaxUint32` é reservado para inválido: IDs válidos vão de 0 a `math.MaxUint32-1`; uma tabela pode ter no máximo `math.MaxUint32` entradas, sujeita à memória disponível. Contagens, offsets e somas de tamanhos usam `uint64`, com conversão para `int` e limites de slice validada na plataforma.

Labels e chaves são **StringIDs do mesmo dicionário**, não exigem espaços/tabelas independentes. Um ID interno só tem sentido dentro de um Graph: nunca reutilizá-lo automaticamente em outro snapshot.

IDs externos são comparados como bytes UTF-8, sem case-folding, locale ou normalização Unicode. A string vazia pode ser ID quando explicitamente representada no CSV. Strings da API são strings Go válidas, inclusive vazias. Não são terminadas por NUL e não devem manter aliases inseguros para bytes que possam ser unmapped.

## Valores

Tags fixas do contrato: `Bool=1`, `Byte=2`, `Short=3`, `Int=4`, `Long=5`, `Float=6`, `Double=7`, `String=8`, `Date=9`, `Datetime=10`.

Inteiros são assinados nas respectivas faixas. Float e Double preservam os bits IEEE 754 após conversão. NaN é normalizado para um quiet-NaN positivo único por tipo; os dois zeros com sinais diferentes permanecem distintos. Strings, Date e Datetime usam strings internadas. Datas são validadas lexicalmente pelo adapter, mas **não são convertidas para epoch** nem comparadas como instantes pelo runtime.

Igualdade é `mesma tag + mesmo payload canônico`. `Int(1)` difere de `Long(1)`, e duas datas textualmente diferentes não se tornam iguais por timezone. Para ordenar conjuntos determinísticos, usar `(key_sid, type_tag, payload_u64)` em ordem crescente; essa ordem **não é uma ordenação numérica de consultas**.

Não persistir cardinalidade declarada: ela é usada para validar e consolidar a carga. Um node pode ter vários pares `(key,value)`; uma edge no máximo um valor por chave. Uma declaração `set` pode unir tipos diferentes da mesma chave. Declarações conflitantes de cardinalidade, ou múltiplos valores em `single`, removem a propriedade inteira conforme INGEST.md.

## Canonicalização

Depois de resolver registros/conflitos:

1. Ordenar nodes e edges sobreviventes pelo ID externo; atribuir IDs internos nessa ordem.
2. Coletar somente strings efetivamente usadas no graph e nas opções de índice. Sempre incluir a string vazia como StringID 0; deduplicar e ordenar lexicograficamente por bytes.
3. Remapear labels, chaves e valores textuais para o dicionário definitivo. Ordenar e deduplicar os labels por node.
4. Ordenar e deduplicar propriedades por `(key_sid, tag, payload)` em cada dono.
5. Produzir CSR, índices e seções na ordem definida no snapshot.

Interner temporário não define identidade persistente. O builder não retém slices de células reutilizados pelo decoder. Copiar os dados necessários; usar strings Go normais, sem conversão unsafe. Para evitar reter registros enormes por substrings curtas, conferir retenção durante os testes de memória.

O mesmo multiconjunto de contribuições, com as mesmas opções, produz os mesmos bytes, independentemente da ordem de enumeração/linhas. Para dados coerentes, reparticionar arquivos sem mudar a semântica de headers também preserva o resultado. Para um CSV quebrado, mudar a posição da quebra pode alterar o prefixo recuperável: isso não viola o determinismo definido.

Paths, datas de build, contadores de linhas, ordem de mensagens e tempos não entram no snapshot. Somente um bit `partial_load` registra perda conhecida de dados. Acrescentar registros rejeitados pode mudar esse bit mesmo quando o grafo sobrevivente é igual.

## Arrays e CSR

Arrays principais:

```text
node_external_ids[N]          # StringID
node_label_offsets[N+1]       # offsets em node_labels
node_labels[L]                # StringID
node_property_offsets[N+1]    # offsets em node_properties
node_properties[Pn]           # registros tipados de 16 bytes no arquivo

edge_external_ids[E]          # StringID
edge_sources[E]               # NodeID
edge_targets[E]               # NodeID
edge_labels[E]                # StringID
edge_property_offsets[E+1]
edge_properties[Pe]

forward_offsets[N+1]
forward_neighbors[E]          # target
forward_edges[E]              # EdgeID
reverse_offsets[N+1]
reverse_neighbors[E]          # source
reverse_edges[E]              # mesma identidade lógica
```

Dentro da adjacência de cada node, ordenar por `(neighbor_id, edge_label_sid, edge_id)`. Uma edge aparece exatamente uma vez em cada CSR. A entidade edge fica em sua tabela; a duplicação de vizinhos nos índices é deliberada para não exigir lookup de edge a cada expansão BFS.

O `graph.Graph` contém colunas válidas e um recurso de backing privado, quando necessário. A implementação pode ter arrays heap nativos e arrays persistentes acessados por helpers equivalentes, mas as consultas não devem conhecer o backend. Não transformar o graph em objetos ligados por ponteiros.

## Índices

Obrigatórios: lookup de IDs externos de nodes/edges por busca binária na ordem canônica; postings por label de node e por label de edge. Cada posting é uma lista crescente de IDs sem duplicatas.

O builder recebe zero ou mais chaves em `--index-property KEY`. Para cada chave escolhida, indexa igualdade de valores tipados de **nodes e edges**. Persistir a lista das chaves pedidas, inclusive as que não possuem valores. Isso distingue “índice vazio” de “índice ausente”.

O índice de propriedade é uma tabela ordenada de `(owner_kind, key_sid, tag, payload, start, count)` e listas de IDs. Um node com valores set pertence a vários postings. O mesmo node não aparece duas vezes no mesmo posting. Consultas sem índice fazem scan; **o resultado é idêntico**. Não há estatísticas ou planner.

Bitmaps ficam nos resultados/scratch, com universo e tamanho explícitos. Nunca usar um bitmap de nodes como conjunto de labels ou edges. AND/OR exigem o mesmo universo.

## Invariantes essenciais

Contagens batem com arrays; offsets começam em zero, são monotônicos e terminam no tamanho da seção referenciada. Toda referência é válida. Todos os labels são strings não vazias. IDs externos são únicos em cada espaço. Edge não existe sem os dois endpoints. Labels/conjuntos/postings não têm duplicatas. Conflitos não deixam valores arbitrariamente escolhidos.

A validação do Graph em memória e do snapshot cobre as mesmas invariantes sem reutilizar cegamente a saída do builder como prova de correção.

## Concretização em Go

Tipos públicos distintos para NodeID, EdgeID e StringID impedem mistura acidental de universos. `NodeSet` e `EdgeSet` retêm a identidade do Graph e não aceitam AND/OR com outro Graph, mesmo se as contagens coincidirem. Um bitmap privado pode usar `[]uint64` sem criar um framework genérico.

O heap inicial usa slices de números. O backend mapeado lê as mesmas colunas lógicas por helpers LE concretos; suas grandes tabelas não são copiadas automaticamente para `[]uint32`. O builder e o codec podem compartilhar `internal/graphdata`, sem expor formato persistente nem imports de infraestrutura ao núcleo. Interfaces não são introduzidas por campo ou acesso de vizinho.

A ordem e os bytes do snapshot são definidos pelo contrato, não por iteração de `map`, arquitetura do host, representação de `int`, ordem de execução de goroutines ou serialização de structs Go.

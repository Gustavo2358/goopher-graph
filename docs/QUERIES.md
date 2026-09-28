# Consultas como funções e programas Go

## Primitiva de alcance

Para um grafo dirigido G e origem v:

`R+(v) = {u | existe percurso dirigido de v até u, incluindo comprimento zero}`.

`R-(v) = {u | existe percurso dirigido de u até v, incluindo comprimento zero}`.

São conjuntos reflexivos: ambos contêm v, inclusive se isolado. O bitmap visited marca a origem **antes** de enfileirar; cada node é expandido no máximo uma vez. Um loop/ciclo não duplica resultado nem provoca recursão infinita.

`query.Reachable` escolhe forward ou reverse CSR. A BFS pode usar um vetor crescente como fila; não precisa de framework de travessias. O filtro opcional de labels de edge determina quais relações podem ser percorridas. Todas as demais características do node são acessíveis para consultas personalizadas implementarem sua própria política.

## Território e antiterritório

`territory(v)` usa R+(v); `anti_territory(v)` usa R-(v). As funções Go devolvem subgrafo com origem incluída. Para a listagem CLI, o padrão é omitir a origem: `R(v) \ {v}`. Mesmo quando um ciclo volta a v, essa regra de apresentação continua removendo a origem. `--include-origin` a mostra.

O formato DOT sempre inclui a origem como âncora para manter os caminhos visíveis. Esta é uma diferença explícita de apresentação, não um algoritmo de alcance diferente. Um node isolado gera listagem vazia (apenas o header CSV), mas DOT com um node.

## Subgrafo completo, não árvore BFS

Para um conjunto de nodes S, selecionar toda edge permitida com source e target em S. Guardar EdgeIDs únicos, não triplas e não apenas predecessores de descoberta.

Em um diamante A->B, A->C, B->Z, C->Z, as quatro edges aparecem. Duas edges diferentes A->B continuam distintas. Em reverse, exportar `A -> B`, não `B -> A`. O índice reverso não altera o modelo.

A construção pode visitar as adjacências de todos os nodes selecionados e checar pertencimento dos endpoints. Não enumerar caminhos para depois juntar listas. Não confundir “visited node” com “já registrar todas as edges incidentes”.

## Região entre A e B

`between(A,B) = R+(A) ∩ R-(B)`, calculados com **o mesmo filtro de edges**. Exportar o subgrafo induzido por esse conjunto e pelo filtro.

Se B não é alcançável a partir de A, resultado vazio. Se é alcançável, A e B pertencem ao resultado. Quando A=B, a região é a componente fortemente conexa que contém A (sem precisar implementar outro algoritmo).

**Isso não é a união exata de todos os caminhos simples em grafos cíclicos.** Exemplo: A->X, X->A, A->B. X pertence à interseção porque pode estar no passeio A->X->A->B, mas não em um caminho simples A->B. O produto chama isso de região de alcançabilidade e não promete resolver outro problema.

## Filtros

Filtros de propriedade/label de node são operações de conjunto disponíveis na API; não viram uma linguagem de expressões na CLI. Uma consulta pode intersectar o território com nodes de `sigla=CO`, mas isso é **filtrar saída**. Impedir a travessia por nodes de outra sigla é outra semântica e deve ser explicitamente programada na consulta, usando os iteradores.

Nenhum cutoff oculto, limite de número de caminhos ou heurística de completude. As consultas iniciais calculam alcance completo do snapshot aceito. Em snapshot parcial, são completas **sobre esse snapshot**, não sobre os dados rejeitados do usuário.

## Extensão

O exemplo [shared_targets.go.txt](../examples/shared_targets.go.txt) calcula a interseção dos nodes alcançáveis de A e B. Ele usa somente a API pública e não é mais um requisito de negócio da engine. O teste externo deve compilá-lo e executar um caso sem editar `graph/` nem criar opção no executor.

Uma consulta incomum pode usar `Graph.Adjacent` e `EdgeIterator.Next`, uma fila/pilha própria e estados auxiliares. Primitivas de alto nível não são a única entrada. Não adicionar um “plugin manager” para essas funções.

## DOT

Writer recebe Graph + Subgraph + `io.Writer`. Emitir `digraph G`, **não `strict digraph`**; não usar `concentrate=true`. Um statement por EdgeID mantém paralelas. [D1](REFERENCES.md)

Nodes têm identificadores sintáticos `n<id_interno>`, em ordem de NodeID. Label visual: ID externo seguido de labels do node, em ordem canônica. Edges são emitidas em ordem de EdgeID, com endpoints lógicos, label de relação, atributo `id="e<id_interno>"` e tooltip de ID externo. O writer não inventa IDs externos vazios para elementos.

Escapar aspas e backslashes. LF/CR viram escapes DOT adequados; outros controles são renderizados como texto visível `\\xHH`, nunca bytes crus que possam quebrar a sintaxe. Não tratar conteúdo do usuário como HTML label nem como atributo DOT executável. Os nomes sintáticos gerados não vêm diretamente dos IDs do CSV.

DOT é visualização resumida: não serializa todas as propriedades de maneira interoperável. Propriedades completas continuam no snapshot/API. Graphviz é ferramenta externa opcional para renderizar/verificar sintaxe; a engine não o chama.

## Cancelamento e alocações

BFS usa fila `[]NodeID` e bitmap próprios da chamada. Nada de goroutine por vizinho, channels por edge ou maps de strings no hot path. Testar cancelamento cooperativo em loops grandes; nunca devolver um prefixo como resposta final completa. Reuso de workspace é otimização futura, não contrato global.

O custo de expansão é O(Vr+Er), mas inicializar um bitmap com universo N custa O(N/64) e O(N) bits; a fila usa O(Vr). Selecionar subgrafo pode inspecionar adjacências de membros para checar endpoints, não só as edges finais. Exportar IDs/strings e ordenar output também tem custo próprio. Não usar o custo da BFS como descrição do comando CLI inteiro.

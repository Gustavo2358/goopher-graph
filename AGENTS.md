# AGENTS — GopherGraph

## Objetivo e fonte da verdade

Construa o motor Go descrito nesta pasta. É um projeto novo. Este pacote e instruções posteriores do usuário são o contrato; não importar decisões de projetos/rascunhos anteriores. Comece por SCOPE, PROGRESS, BACKLOG e ARCHITECTURE. Leia os outros documentos conforme a próxima fatia exige.

O prompt inicial é deliberadamente curto. Não transformar cada sessão em um plano enorme nem reproduzir todas as specs no output.

## Limites que não podem regredir

- Packages por capacidade. Ports/adapters locais; nenhum pacote horizontal global `application`, `domain` ou `infrastructure`.
- Core de grafo e consultas não importam filesystem, mmap, CSV Neptune, CLI, HTTP ou AWS. Interfaces nas fronteiras reais, não em todo algoritmo.
- Nodes e edges são catálogos obrigatoriamente separados. Terminar todos os nodes antes de consumir registros de edge. Arquivo no catálogo errado é rejeitado, não reclassificado.
- Header inválido rejeita fonte; registro delimitado inválido rejeita registro; endpoint inexistente rejeita edge. Diagnosticar e continuar. Não usar `return err` indiscriminadamente para abortar o lote por bad data.
- Delimitação CSV irrecuperável encerra apenas a fonte, preservando registros completos anteriores. Nunca inventar registros usando o próximo newline de um campo multilinha.
- Multigrafo direcionado: IDs de edges, propriedades e paralelas são preservados. Nenhum enum de negócio, placeholder de node ou deduplicação pela tripla.
- Runtime imutável. Snapshot novo é publicado sem reescrever o inode de leitores ativos. Go e `MAP_PRIVATE` não protegem contra truncate externo ou unmap prematuro.
- Queries são funções Go reutilizáveis, com acesso às adjacências. Sem DSL, planner, registry, servidor, framework de plugins ou transporte remoto nesta entrega.

## Loop Lean SDD

Escolha uma fatia liberada, identifique o comportamento de aceite, escreva um teste que detecta o atalho errado, implemente e rode regressões. Feche com evidência curta e avance. Uma fatia ativa basta. Detalhes internos podem ser decididos pelo agente; não pedir aprovação para cada função, arquivo ou gate verde.

Não mudar fixtures para esconder bugs. Corrigir uma inconsistência concreta de spec exige explicação e um exemplo, não cerimônia. Não promover uma ideia futura a bloqueio retroativo do fechamento.

## Disciplina Go

Um módulo; stdlib primeiro. Use `map`, slices, `io.Reader`/`io.Writer`, `context`, `encoding/binary`, `strconv`, `testing` e ferramentas Go. Não reimplementar hashmap, vetor dinâmico, allocator ou vtable C. Não criar interfaces de uma implementação por estética. As interfaces de transporte têm valor real porque filesystem e cloud podem variar.

`golang.org/x/sys/unix` é a única dependência externa prevista, isolada em adapter Linux. Usar o módulo disponibilizado/aprovado pelo ambiente. `go.mod` e `go.sum` devem ser mantidos normalmente. Não contornar políticas corporativas, exigir pins de patch, SHAs de documentação ou fazer download escondido no teste.

Produção deve compilar com `CGO_ENABLED=0`. `go test -race` é outra execução: requer ambiente compatível e cgo para a instrumentação; isso não adiciona cgo ao produto. Não marcar esse gate como executado em um ambiente que não o oferece.

Hot path usa IDs, arrays/colunas compactas e bitsets, não objetos por edge nem `map[string]any`. Não expor slices graváveis sobre o snapshot. Começar sem `unsafe` no código próprio; leitores LE podem acessar o mmap sem copiar arrays inteiros. Otimização insegura não é pré-requisito do produto.

`error` comunica falhas operacionais; diagnósticos tipados comunicam rejeições recuperáveis. Não usar `log.Fatal`, `os.Exit`, panic/recover ou comparação de mensagens como fluxo normal em bibliotecas. Não prometer recuperar OOM fatal do runtime Go. Limitar inputs e checar aritmética antes de alocar ou indexar.

Liberar arquivos, mappings e staging explicitamente. `defer` de fechamento pertence à função que processa uma fonte, não ao loop inteiro de milhares de arquivos. Não confiar no GC/finalizer para unmap. Não compartilhar workspace mutável entre consultas.

## Testes e honestidade

Executar os testes anunciados. Targets vazios, mocks exclusivos e stubs não são funcionalidade. Os artefatos de `spec/` e o oráculo Python verificam contratos; não contam como uma engine Go implementada.

Testes ficam próximos ao package; E2E usa `testing`, `t.TempDir` e `os/exec`. Table tests, fontes fragmentadas, erros de I/O, round-trip, goldens independentes, fuzz e `-race` são usados onde agregam evidência. Não é necessário framework externo de testes.

Separar tempo de build, abrir/validar, consulta e exportação. Não prometer latência nem superioridade sobre C/Java sem benchmark equivalente. Medir RSS/mapping além do heap Go.

## Estado e fechamento

Só `PROGRESS.md` guarda andamento: pendente, em andamento, concluído ou bloqueado com causa concreta. Fechar fatias e produto conforme `docs/CLOSEOUT.md`; comando, resultado e próximo passo bastam. Não exigir pins, SHAs, certificados, múltiplos relatórios iguais ou aprovação formal por wave.

Git local pode ser usado normalmente. Não criar remote, dar push, acessar dados corporativos, publicar em cloud ou alterar sistemas externos sem pedido. Ao fim de B12, encerrar: HTTP, S3 e novas queries são outro escopo.

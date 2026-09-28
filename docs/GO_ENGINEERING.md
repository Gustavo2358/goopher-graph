# Go idiomático, recursos e limites

## Toolchain e dependências

O módulo local inicial pode chamar-se `gophergraph`, sem afirmar que existe um repositório público. Quando houver caminho corporativo real, ele substitui esse nome pelos mecanismos normais do Go. Não inventar remote.

B00 usa a toolchain Go aprovada e disponível. A diretiva `go` registra o requisito técnico mínimo necessário; não adicionar uma diretiva `toolchain` de patch por ritual. Não exigir uma versão do sistema mais nova só porque ela existe. Nenhuma API proposta depende de uma novidade recente. [G4](REFERENCES.md)

Dependência de produto prevista: `golang.org/x/sys/unix`, somente no adapter Linux. Usar a versão compatível disponibilizada no ambiente. Não adicionar petgraph equivalente, framework CLI, DI, logging ou suíte de testes. `go.mod`/`go.sum` e, quando a operação exigir, `go mod vendor`, são práticas normais; a proibição de burocracia não significa apagar a integridade normal do gerenciador.

Não gerar chamadas de rede no build/teste. Dependências devem estar disponíveis pelo mecanismo autorizado; indisponibilidade é uma limitação concreta do ambiente, não autorização para bypass. O pacote documental não inclui fontes de dependências.

## Coleções e valores

Use `map[string]...` para ingestão, `[]uint32`/`[]uint64` e bitsets para travessias, `sort`/`slices` quando disponíveis. Não implementar hashmap/vector/interner genérico por tradução do desenho C. Um interner específico ainda pode existir como map + lista, porque canonicalizar strings é parte do produto.

Não usar `any` para cada propriedade/edge no runtime. Tags e payloads fixos simplificam tipos, igualdade e formato. `map` não define ordem persistente: ordenar explicitamente. Não comparar NaN usando `==`; aplicar igualdade canônica da spec.

## CSV não é apenas ReadAll

`encoding/csv` fornece parsing, mas retorna strings sem indicar se o campo era quoted e normaliza CRLF em conteúdo multiline. [G1](REFERENCES.md) Neptune diferencia campo ausente de string vazia explicitamente escrita. [N1](REFERENCES.md)

O adapter conserva presença/aspas e fronteiras do registro, impõe limites e complementa a stdlib. Não usar `ReadAll` em cargas grandes nem `bufio.Scanner` com seu limite default para CSV multilinha. Não usar `LazyQuotes` para aceitar dados quebrados silenciosamente. O detalhamento e os casos de aceite estão em INGEST.

## Erros e cancelamento

Funções retornam `error` para operação que não pôde ser concluída. Rejeição de registro é evento normal de carga, não um erro fatal global. Use sentinelas/erros tipados e `errors.Is`/`errors.As`; nunca buscar substrings em mensagens para classificar recuperação.

Use `context.Context` na ingestão, codec de operações longas e consultas. Conferir cancelamento em unidades limitadas de trabalho, inclusive em grafos com um hub de grau enorme. Não é necessário consultar o relógio por edge. Ao cancelar uma query, retornar erro e nenhum resultado aparentemente completo; ao cancelar build, não publicar produto parcial por cancelamento. Perda de dados admitida e cancelamento são coisas distintas. [G7](REFERENCES.md)

Não guardar contexto global nem criar uma goroutine para envolver todo `io.Reader`: o adapter concreto precisa respeitar o contexto quando o transporte permite. Leitura de arquivo local e APIs futuras de rede têm mecanismos próprios de cancelamento.

## mmap não é memória gerida pelo GC

O adapter chama `unix.Mmap`/`unix.Munmap`; as views são byte slices com recurso externo. [G2](REFERENCES.md) O Graph retém um owner/release, e Close encerra o recurso explicitamente. Não confiar em finalizers.

Começar com `binary.LittleEndian.Uint32/Uint64` em bytes validados, sem `unsafe` no código próprio. [G3](REFERENCES.md) Isso mantém o arquivo mapeado sem copiar todas as suas colunas; não equivale a zero instruções por acesso. Se um perfil futuro justificar views tipadas, a proposta fica confinada à fronteira de representação e exige nova evidência, não é gate inicial.

Strings públicas são cópias Go seguras quando extraídas do mapping. Não usar `unsafe.String` ou conversões zero-copy que sobrevivam ao unmap. Iteradores pertencem a um Graph aberto; resultados têm bitsets heap e identidade desse Graph. Não expor backing slices para escrita.

## Concorrência e Close

Graph publicado não muda. Cada query tem fila/visited próprios. Nenhum scratch global nem cache lazy não sincronizado. `Close` é idempotente em chamadas sequenciais, mas não é permitido fechá-lo enquanto consultas/iteradores estão em uso. O dono coordena isso; não implementar um servidor de leases por antecipação.

`go test -race` verifica as execuções exercitadas, não prova ausência universal de races e não torna um mapping truncado seguro. A instrumentação usa cgo em plataformas suportadas, mesmo que o binário de produção use `CGO_ENABLED=0`. [G5, G8](REFERENCES.md)

## Recursos e observabilidade de memória

Validar overflow e conversão uint64 -> int antes de slice/make. Limites por registro/fonte devem ser aplicados antes do crescimento ilimitado de buffers. Out-of-memory fatal do runtime não é um erro comum recuperável com `recover`; não criar allocator público para simulá-lo. Testar limites determinísticos e falhas dos ports reais.

Heap, bytes mapeados, RSS, page faults e tamanho de resultados são medidas diferentes. O limite de memória do runtime não abrange todo recurso obtido manualmente por mmap; não usar apenas `GOMEMLIMIT` como limite do processo/container. [G6](REFERENCES.md)

## Teste e estilo

`gofmt`, `go vet`, `go test`, table tests, fuzz nativo e benchmarks são suficientes. Exemplos e API devem parecer Go, não C com sintaxe diferente: retorno `(valor, error)`, interfaces por comportamento, `io.Reader/io.Writer` nas fronteiras e nenhuma função free para objetos heap comuns. Arquivos/transactions/mappings continuam exigindo Close/Abort.

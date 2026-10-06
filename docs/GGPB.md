# Resultados GGPB v1

**GGPB é um protocolo lógico compacto, não um snapshot e não um dump das
estruturas internas do engine.** O consumidor recupera IDs externos, labels,
propriedades tipadas, topologia e metadata sem abrir o snapshot original.

## Uso

```sh
gophergraph territory --snapshot graph.snapshot --node PROGRAM:A \
  --format ggpb --output result.ggpb
gophergraph anti-territory --snapshot graph.snapshot --node PROGRAM:A \
  --format ggpb --output reverse.ggpb
gophergraph between --snapshot graph.snapshot --from PROGRAM:A --to PROGRAM:B \
  --format ggpb --output between.ggpb
gophergraph decode --input result.ggpb --format json --output result.json
```

Sem `--output`, ambos os comandos escrevem em stdout. Não enviar stdout binário
para um terminal interativo. `ids`, `dot` e `json` conservam seus contratos.
`--include-origin` continua exclusivo de `ids`; GGPB contém o subgrafo completo.
O aviso `PARTIAL` das queries fica em stderr. `decode` não abre um snapshot.

## Fronteiras e mensagens

```text
snapshot / graph → query → Subgraph → iteradores públicos → ggpb.Emit
                                                        │
                                                   pb.Batch
                                                        │
                                 ┌──────────────────────┴─────────────────┐
                            ggpb.Write                            futuro stream.Send
                       framing → io.Writer                         (não implementado)
```

`graph` e `query` não importam GGPB, Protobuf, JSON ou I/O. Não houve alteração
nos sets, no snapshot ou nos algoritmos das queries. O core ganhou apenas
IterateNodeLabels, alternativa sem cópia ao NodeLabels existente. A query termina e
materializa seu Subgraph antes de qualquer serialização. O código atual também
tem WASM; seu caminho de saída JSON permanece preservado.

O contrato público é [result.proto](../ggpb/pb/result.proto); o Go gerado usa o
runtime oficial `google.golang.org/protobuf`, isolado em `ggpb`. A biblioteca
expõe:

```go
err := ggpb.Write(ctx, output, g, sub, ggpb.Query{
    Name: "territory", Node: &origin, EdgeLabels: requestedLabels,
})

err = ggpb.Emit(ctx, g, sub, metadata, ggpb.Options{}, func(b *pb.Batch) error {
    // Consumir sincronicamente; b é emprestado até este callback retornar.
    return transport.Send(b)
})

reader := ggpb.NewReader(input)
for {
    batch, err := reader.Next(ctx)
    if err == io.EOF { break } // só após End válido e EOF físico
    if err != nil { return err }
    consume(batch)
}
err = ggpb.JSON(ctx, output, input) // contrato graphjson, sem snapshot
```

A sequência obrigatória é `ResultHeader`, zero ou mais `Records`, `ResultEnd`.
Cada `Batch` contém exatamente uma dessas variantes. Todos os nodes precedem
as edges. IDs são strings externas; não há IDs de CSR, offsets, bitmaps ou views
mmap no protocolo. Os nodes/edges seguem a ordem crescente de ID externo.

`NodePart`/`EdgePart` permitem uma entidade atravessar batches: a primeira parte
tem `id` **presente**, mesmo quando `""`; continuations omitem `id`. Na primeira
parte de edge, `source`, `target` e `label` são obrigatórios. Continuations não
repetem esses campos. Partes são contíguas, sem intercalar entidades. `last=true`
encerra a entidade. Labels precedem properties; cada lista concatena suas partes.
Um node isolado e um resultado vazio são válidos. Paralelas e loops mantêm IDs
próprios e direção original, inclusive em anti-territory.

Header inclui versão, query, directed, partialSnapshot e contagens uint64. End
repete as contagens efetivamente concluídas. Metadata optional distingue ID vazio
de ausência. `filtered=false` significa filtro irrestrito; `true` com lista vazia
significa nenhuma edge permitida. Filtros são ordenados e sem duplicatas, incluindo
labels desconhecidos. O chamador fornece os parâmetros executados.

## Tipos e dicionário

Cada Property tem chave, enum de tipo e um `oneof` de valor compatível:

| Kind | Campo Protobuf |
|---|---|
| Bool | bool |
| Byte, Short, Int, Long | sint64, com limites do tipo declarados no enum |
| Float | float IEEE-754 de 32 bits |
| Double | double IEEE-754 de 64 bits |
| String, Date, Datetime | string UTF-8, texto armazenado preservado |

Long não passa por float ou string no binário. Float/Double preservam NaN,
±Inf e zero negativo; NaN segue a canonicalização do modelo de grafo. Properties
set têm uma entrada por valor, sem converter para map nem perder chaves repetidas.
A ingestão não retém anotações de cardinalidade: GGPB preserva os valores que
existem no grafo, como JSON. Date/Datetime não são convertidos em timestamps.

`Symbol` vale para labels e property keys. `ref=0` usa `text` inline (inclusive
vazio). `ref>0` é índice **1-based no dictionary deste Records**, e `text` deve
ser vazio. O dictionary reinicia em cada batch; pode conter até 1.024 strings e
64 KiB de texto UTF-8. Após o limite, novos símbolos ficam inline. Não há mapa
proporcional ao resultado nem cópia do dicionário físico do snapshot. As strings
são internadas pela ordem determinística de primeira ocorrência no batch.

`Options{InlineSymbols:true}` produz uma variante válida sem dictionary para
medir a alternativa. A campanha em [GGPB_BENCHMARKS](GGPB_BENCHMARKS.md) compara
as duas. Textos de propriedades e IDs externos não são internados.

## Framing de arquivo

O wire format das mensagens é o [Protobuf oficial](https://protobuf.dev/programming-guides/encoding/).
O adapter de arquivo adiciona:

```text
8 bytes magic: 47 47 50 42 0d 0a 1a 0a (GGPB\r\n\x1a\n)
repetir:
    uint32 little-endian: N (1 <= N <= 4 MiB)
    N bytes: mensagem pb.Batch
    uint32 little-endian: CRC32 IEEE dos N bytes de payload
EOF imediatamente após o frame ResultEnd
```

Os comprimentos e checksum pertencem ao framing, não ao snapshot. Endianness dos
valores nas mensagens segue Protobuf. CRC32 detecta corrupção acidental; não é
assinatura/autenticação. Truncamento de qualquer prefixo, payload, checksum ou
End falha. EOF antes de End não representa um resultado parcial válido. Bytes
após End falham, inclusive outro arquivo concatenado. Não há footer aleatório,
compressão geral, indexação, seek obrigatório ou necessidade de ler o arquivo todo.

## Limites, memória e erros

Encoder limita batches por um orçamento conservador de 256 KiB ou 256 partes.
Partes normalmente usam um orçamento conservador de 32 KiB. O escalar corrente
pode ultrapassar esses alvos e é emitido sem reparticionar a string; cada string
UTF-8 é limitada a 1 MiB, e cada Batch codificado a 4 MiB. Metadata também deve
caber num frame. Um conjunto de IDs/label grande demais para a primeira parte
pode atingir o limite do frame mesmo com escalares individualmente permitidos.
Nesses casos retorna `ErrLimit`; não descarta nem altera valores silenciosamente.
Esses são limites explícitos do perfil v1, não restrições novas ao grafo ou JSON.

A memória auxiliar do encoder depende de um batch, um dictionary, uma parte,
metadata limitada. `Graph.IterateNodeLabels` foi a única extensão mínima do
core: o NodeLabels anterior copia a lista inteira e não permitiria garantir memória
limitada para um único node com muitos labels. Ambos conservam a mesma ordem e
semântica; a API anterior permanece intacta. Labels e properties usam iteradores.
O buffer de marshaling retém no máximo o maior frame. O destino deve consumir os
bytes incrementalmente: escolher bytes.Buffer acumula a saída por decisão do
chamador. TotalAlloc cresce com trabalho realizado; heap vivo auxiliar não deve
crescer com o tamanho total do resultado. Há teste em duas escalas com >214 MB de
saída e uma entidade com 30 mil propriedades dividida em partes.

Reader retém um buffer de frame e estado escalar de ordem/contagem. Cada Next
retorna uma mensagem independente; retenção pelo consumidor é escolha dele.
Objetos Protobuf decodificados têm overhead além dos bytes de frame. Reader
valida magic, tamanho antes da alocação do payload, checksum, Protobuf, versão,
metadata, tipos/limites de valores, referências de dictionary, ordem/unicidade de
IDs, continuidade, contagens e End. Não faz validação global de pertencimento dos
endpoints ao conjunto de nodes; isso exigiria um índice proporcional ao resultado.
O encoder garante esse pertencimento usando o Subgraph válido.

`ErrInvalid`, `ErrVersion`, `ErrLimit`, erros de I/O e cancelamento são observáveis
por `errors.Is`. Qualquer falha deixa um prefixo descartável; não há rollback nem
publicação atômica do arquivo de resultado. Writer não fecha o destino; Reader não
fecha a origem. Short writes falham. Contexto é checado nas iterações e entre
escritas/leituras; não interrompe uma chamada I/O bloqueada dentro de um port.
O chamador mantém graph aberto e dados/parâmetros sem mutação até terminar.

## Compatibilidade, determinismo e evolução

Reader aceita versão **1** e rejeita qualquer outra como `ErrVersion`. Campos
aditivos desconhecidos do schema são ignorados conforme Protobuf. Novas variantes
obrigatórias, mudanças de interpretação, limites ou framing incompatíveis exigem
nova versão. Números de fields existentes não serão reutilizados. Um payload
Batch desconhecido não equivale a records vazios e é rejeitado. Não há migrations.

A implementação v1 fixa ordem das mensagens, entidades, símbolos e valores, usa
marshaling determinístico e não inclui timestamps. Mesmo snapshot, query,
parâmetros, opções do encoder e versão produzem os mesmos bytes; filtros
semanticamente equivalentes também. Protobuf não promete um encoding canônico
entre todas as implementações/languages: consumidores devem comparar semântica.
`decode --format json` usa o contrato [JSON](JSON.md), não ProtoJSON; nos testes,
seus bytes são iguais aos do graphjson nas fixtures e casos adicionais.

Emit separa os batches lógicos do arquivo. Um futuro adapter gRPC poderá passar
cada pb.Batch diretamente a stream.Send; não precisará gerar/ler um arquivo.
Não há gRPC, HTTP, Lambda, S3 ou streaming durante traversal nesta entrega.

## Regenerar e validar interoperabilidade

Go gerado está versionado; protoc não é dependência de build/teste do produto.
Foi usado protoc 35.1 (grpcio-tools) com protoc-gen-go 1.34.2, que gera código sem
unsafe e é compatível com o runtime Go 1.36.12 usado pelo módulo.

```sh
protoc -I . --go_out=. --go_opt=paths=source_relative ggpb/pb/result.proto
# Instalar/selecionar protoc-gen-go 1.34.2 antes de regenerar sem unsafe.
protoc -I ggpb/pb --python_out=/tmp/ggpb-python ggpb/pb/result.proto
PYTHONPATH=/tmp/ggpb-python python3 tools/ggpb_python.py result.ggpb
```

O consumidor Python opcional precisa do pacote oficial protobuf e do módulo
result_pb2 gerado. Ele verifica framing, valores oneof, dictionaries, continuidade
e contagens incrementalmente, e imprime contagens/tipos. Não é uma dependência
operacional da engine nem um validador completo de entradas hostis.

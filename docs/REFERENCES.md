# Referências técnicas

Fontes primárias consultadas para o desenho em Go. As decisões de produto são as deste pacote; links não alteram silenciosamente o contrato. Não há dependência de rede para executar testes locais.

## Formato de entrada

- **N1 — Neptune Gremlin CSV:** https://docs.aws.amazon.com/neptune/latest/userguide/bulk-load-tutorial-format-gremlin.html — headers, tipos, blank/empty, multilabel e cardinalidade.
- **N2 — Neptune loader:** https://docs.aws.amazon.com/neptune/latest/userguide/load-api-reference-load.html — contexto do formato `csv` e distinção do serviço de carga.

## Go

- **G1 — encoding/csv:** https://pkg.go.dev/encoding/csv — Read, ParseError, ReuseRecord, normalização CRLF; o retorno não preserva quotedness.
- **G2 — x/sys/unix:** https://pkg.go.dev/golang.org/x/sys/unix — Mmap/Munmap e operações Unix do adapter.
- **G3 — encoding/binary:** https://pkg.go.dev/encoding/binary — leitura/escrita de campos LE de largura fixa.
- **G4 — go.mod:** https://go.dev/doc/modules/gomod-ref — module, go, require e toolchain.
- **G5 — race detector:** https://go.dev/doc/articles/race_detector — execução e requisitos de instrumentação.
- **G6 — guia de GC:** https://go.dev/doc/gc-guide — heap, recursos externos e limites de memória do runtime.
- **G7 — context:** https://pkg.go.dev/context — cancelamento/deadlines propagados por chamadas.
- **G8 — memory model:** https://go.dev/ref/mem — sincronização e responsabilidade sobre acesso concorrente.
- **G9 — strconv:** https://pkg.go.dev/strconv — ParseInt/ParseFloat; sua gramática não substitui o perfil Neptune.
- **G10 — fuzzing:** https://go.dev/doc/security/fuzz/ — fuzz nativo em testes Go.
- **G11 — gopher:** https://go.dev/blog/gopher — referência cultural para o nome GopherGraph; este pacote não usa imagem do mascote.

## Sistema e exportação

- **P1 — mmap:** https://man7.org/linux/man-pages/man2/mmap.2.html — mapping, proteção e riscos por alteração/truncamento.
- **P2 — rename:** https://man7.org/linux/man-pages/man2/rename.2.html — publicação por substituição de nome.
- **P3 — fsync:** https://man7.org/linux/man-pages/man2/fsync.2.html — sincronização de arquivo e diretório.
- **D1 — DOT:** https://graphviz.org/doc/info/lang.html — digraph, paralelas e quoting.

## O que é política local, não promessa do Neptune

Carga tolerante por registro/fonte; remoção determinística de propriedades conflitantes; quarentena de EdgeID estruturalmente inconsistente; warning na coerção Bool; datas como texto validado; distinção entre origem incluída na API e omitida na listagem. Nem os documentos nem os testes afirmam equivalência transacional com o serviço ou comparação de performance com outro motor.

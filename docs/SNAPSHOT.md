# Snapshot binário e publicação

Este é o layout inaugural a implementar. Não existe arquivo de produto anterior a migrar. `format_version=1` serve exclusivamente para identificação técnica do formato.

## Regras gerais

Little-endian; bytes de 8 bits; arquivo menor ou igual a `math.MaxInt64` e representável como `int` na plataforma Linux/amd64. Todas as posições/tamanhos são offsets/contagens sem sinal. Nunca persistir ponteiros, headers de slices/strings, structs Go nativas, padding ou `int` dependente da plataforma.

Header de 64 bytes, seguido imediatamente por 24 entradas de diretório de 32 bytes. Todas as 24 seções estão presentes, mesmo vazias, em ordem crescente de ID. A primeira seção começa em `align64(64 + 24*32) = 832`. Cada seção seguinte começa em `align64(offset_anterior + bytes_anteriores)`. O arquivo termina em `align64(fim_da_última_seção)`. Todo padding/reservado é zero. Seção vazia pode ter o mesmo offset que a seguinte.

`align64(x) = (x+63) & ~63`, somente após checar overflow da soma. Uma seção de zero elementos tem byte_length zero, não uma tabela inventada de placeholders.

Leitura de campos escalares usa `encoding/binary.LittleEndian`, não reflection/gob nem casts de structs sobre o mmap. Arrays podem ter uma view interna eficiente, mas o produto não depende de aliasing/alinhamento acidental. As interfaces públicas não expõem posições de seções.

## Header

| Offset | Bytes | Campo | Valor/restrição |
|---:|---:|---|---|
| 0 | 8 | magic | bytes `47 4f 50 48 47 52 50 48` (ASCII `GOPHGRPH`) |
| 8 | 4 | format_version | 1 |
| 12 | 4 | header_size | 64 |
| 16 | 8 | flags | bit 0 = partial_load; demais bits zero |
| 24 | 8 | file_size | tamanho físico exato, incluindo padding |
| 32 | 8 | node_count | N, até math.MaxUint32 |
| 40 | 8 | edge_count | E, até math.MaxUint32 |
| 48 | 4 | string_count | S, de 1 a math.MaxUint32 |
| 52 | 4 | section_count | 24 |
| 56 | 8 | directory_offset | 64 |

Endianness é parte do formato, não um flag negociável. Um reader de plataforma não suportada retorna erro tipado de plataforma não suportada, não tenta reinterpretar o arquivo silenciosamente.

## Entrada de diretório

Offset relativo à entrada: `+0 section_id:u32`, `+4 element_size:u32`, `+8 file_offset:u64`, `+16 byte_length:u64`, `+24 element_count:u64`.

Checar `count * element_size == byte_length` sem overflow, alinhamento e intervalo dentro do arquivo. Seções não podem sobrepor header/diretório ou outras seções não vazias. A forma canônica acima fixa também a posição esperada de cada seção.

## Seções

| ID | Nome | Elemento | Contagem |
|---:|---|---:|---|
| 1 | STRING_OFFSETS | u64 | S+1 |
| 2 | STRING_BYTES | byte | B |
| 3 | NODE_EXTERNAL_IDS | u32 | N |
| 4 | NODE_LABEL_OFFSETS | u64 | N+1 |
| 5 | NODE_LABELS | u32 | L |
| 6 | NODE_PROPERTY_OFFSETS | u64 | N+1 |
| 7 | NODE_PROPERTIES | PropertyValue, 16 bytes | Pn |
| 8 | EDGE_EXTERNAL_IDS | u32 | E |
| 9 | EDGE_SOURCES | u32 | E |
| 10 | EDGE_TARGETS | u32 | E |
| 11 | EDGE_LABELS | u32 | E |
| 12 | EDGE_PROPERTY_OFFSETS | u64 | E+1 |
| 13 | EDGE_PROPERTIES | PropertyValue, 16 bytes | Pe |
| 14 | FORWARD_OFFSETS | u64 | N+1 |
| 15 | FORWARD_NEIGHBORS | u32 | E |
| 16 | FORWARD_EDGES | u32 | E |
| 17 | REVERSE_OFFSETS | u64 | N+1 |
| 18 | REVERSE_NEIGHBORS | u32 | E |
| 19 | REVERSE_EDGES | u32 | E |
| 20 | LABEL_INDEX_ENTRIES | LabelEntry, 24 bytes | Glabel |
| 21 | LABEL_POSTINGS | u32 | L+E |
| 22 | INDEXED_PROPERTY_KEYS | u32 | K |
| 23 | PROPERTY_INDEX_ENTRIES | PropertyEntry, 32 bytes | Gprop |
| 24 | PROPERTY_POSTINGS | u32 | Qprop |

Quantidades sem campo no header vêm das respectivas entradas de diretório. Usar contagem de ELEMENTOS em offsets de propriedades/labels/CSR/postings, não bytes.

### Strings

`STRING_OFFSETS[0]=0`; `STRING_OFFSETS[S]=B`. A string i ocupa `[offset[i], offset[i+1])`, sem terminador. StringID 0 é sempre vazia; as demais são não vazias, únicas e ordenadas por bytes. Logo `offset[0]=offset[1]=0`; outras diferenças são positivas. B pode ser zero em um graph vazio.

IDs externos nos arrays 3 e 8 estão estritamente crescentes em StringID, pela mesma ordem usada para atribuir NodeID/EdgeID. Labels e keys referenciam strings não vazias.

### PropertyValue — 16 bytes

| Offset | Tipo | Campo |
|---:|---|---|
| 0 | u32 | key_sid |
| 4 | u8 | type_tag |
| 5 | u8 | flags = 0 |
| 6 | u16 | reserved = 0 |
| 8 | u64 | payload |

Payload: Bool é 0/1; Byte/Short/Int/Long são complemento de dois com extensão de sinal a 64 bits, validado na faixa da tag; Float usa os 32 bits inferiores e zeros nos superiores; Double usa 64 bits; String/Date/Datetime guardam StringID nos 32 bits inferiores e zeros nos superiores.

NaN canônico: Float `0x7fc00000`; Double `0x7ff8000000000000`. Demais codificações NaN são não canônicas. Zero negativo é preservado. Datas continuam texto; formato de data é responsabilidade da ingestão, e o reader valida tag/referência/UTF-8, não recalcula calendário no hot path.

Em cada dono, ordenar por `(key_sid,type_tag,payload)`; rejeitar duplicatas. Em edge, não pode haver duas entradas de mesma key, mesmo que tipos/valores diferentes. Um node pode ter várias.

### CSR

Offsets começam em zero e terminam em E. Cada EdgeID deve aparecer uma vez em cada sentido. Na lista forward do node u, `edge_sources[id]=u` e `edge_targets[id]=neighbor`. Na reverse de v, `edge_targets[id]=v` e `edge_sources[id]=neighbor`. Listas ordenadas por `(neighbor,label_sid,edge_id)`.

Self-loop aparece uma vez em forward e uma vez em reverse; não duas vezes em cada um. Aresta paralela possui outro EdgeID e continua presente.

### LabelEntry — 24 bytes

`+0 owner_kind:u32` (0=node, 1=edge); `+4 label_sid:u32`; `+8 posting_start:u64`; `+16 posting_count:u64`.

Ordenar por `(owner_kind,label_sid)`. Cada posting é crescente e único. Cada label de node corresponde a um membership; cada edge a um membership. Não guardar grupos vazios. Starts são contíguos, primeiro zero, fim total L+E.

### PropertyEntry — 32 bytes

`+0 owner_kind:u8`; `+1 type_tag:u8`; `+2 reserved:u16=0`; `+4 key_sid:u32`; `+8 payload:u64`; `+16 posting_start:u64`; `+24 posting_count:u64`.

Ordenar por `(owner_kind,key_sid,type_tag,payload)`. Payload tem exatamente a semântica da PropertyValue. Somente keys presentes em INDEXED_PROPERTY_KEYS podem gerar entradas. Keys escolhidas são crescentes, únicas e não vazias; podem não gerar posting. Os starts são contíguos. Postings são crescentes e únicos dentro de cada grupo, com IDs no universo correto.

Qprop é o número de pares únicos `(owner_kind,entity_id,key,type,payload)` de propriedades indexadas. Isso permite validar cobertura integral, não apenas consistência dos IDs existentes.

## Validação de abertura

Antes de devolver Graph:

1. Tamanho mínimo, magic, formato, flags, header, limites de plataforma e diretório canônico.
2. Multiplicações/intervalos sem overflow; padding zero; tamanhos/counts cruzados.
3. Strings ordenadas, UTF-8 válido e referências; IDs externos únicos/crescentes; labels não vazios.
4. Offsets e propriedades por dono; payloads/tag/reservados e ordenações válidos.
5. CSR: contagens, ranges, ordem, direção e ocorrência exata de cada edge em cada índice. Bitmap temporário de edges pode verificar cobertura.
6. Índices: chaves/grupos ordenados, postagem única, todos os memberships publicados são verdadeiros; totais correspondem ao modelo. Validar ausência de memberships faltantes, não só faixa de IDs.

Falha retorna `ErrCorruptSnapshot`, libera recursos adquiridos e não entrega Graph parcial. Não existe modo “confiar nos offsets” no produto inicial. Não indexar/slicar primeiro para verificar depois; panic não é o mecanismo de validação.

A validação percorre o conteúdo relevante e pode fazer buscas para conferir índices; tem custo proporcional aos dados e aos checks executados, não O(1). Medir separadamente de BFS. O alinhamento de 64 bytes simplifica o layout; não é uma promessa de ganho de cache.

Não há checksum criptográfico. Validação detecta corrupção estrutural/semântica do formato, não autentica o arquivo nem detecta toda alteração que produz outro graph válido. Permissões e imutabilidade do snapshot são parte da operação.

## mmap e lifetime

O adapter local abre arquivo regular somente leitura, consulta seu tamanho e mapeia com proteção `PROT_READ` e flag `MAP_PRIVATE`, em seus parâmetros correspondentes. Usar `unix.Mmap` e conferir o `error`; liberar com `unix.Munmap`. O descriptor pode ser fechado depois do mapping bem sucedido; o owner retém o slice original até Close. Não dar Munmap em subslice. [G2, P1](REFERENCES.md)

MAP_PRIVATE não congela o arquivo de origem contra writes/truncate de outro processo. Arquivo truncado pode causar SIGBUS apesar de ter sido validado. Não prometer resolver isso com validação posterior. Contrato operacional: nunca modificar os bytes/inode publicado enquanto houver leitores. Se não puder garantir, materializar uma cópia privada em heap por outro source adapter; isso não é a estratégia local principal.

Um objeto S3 não é um arquivo diretamente mapeável. Um adapter futuro deve materializar bytes estáveis antes de cumprir SnapshotSource.

## Publicação local

Um writer por destino é responsabilidade operacional; não implementar serviço de lock/distribuição.

Criar tempfile exclusivo no diretório do destino, inicialmente modo 0600. Escrever sequencialmente todos os bytes; seal disponibiliza uma view para validar. Somente depois de a validação passar: liberar a view, sincronizar o tempfile, fechar handles que precisem ser fechados, fazer rename sobre o nome final e sincronizar o diretório. O tempfile e o destino devem estar no mesmo filesystem. [P2, P3](REFERENCES.md)

Antes de rename, uma falha apaga somente staging e deixa o arquivo anterior intacto. Após rename, leitores já mapeados continuam com o arquivo antigo; novas aberturas veem o novo. Falha no fsync do diretório **não desfaz rename**: retornar `PublishedUncertain` e erro operacional; não remover o destino novo nem alegar preservação do anterior.

Não colocar output dentro dos diretórios de input; o adapter CLI valida essa configuração para não ingerir snapshots/logs em execuções futuras. Não reusar um input como output. Rename/sync não são transações de graph nem atualizações incrementais.

## Referências executáveis

`spec/snapshot_layout.json` espelha constantes, strides e counts. `tools/reference_snapshot.py` codifica modelos pequenos já normalizados; **não lê CSV nem implementa o motor**. Os `.snapshot` em `fixtures/snapshot_reference/` são goldens do contrato e devem ser comparados byte a byte, sem pins ou hashes de release. Testar também writer Go contra decoder independente e reader Go contra goldens, para evitar um reader e writer errados que concordam entre si.

## Ownership e segurança específicos de Go

O port de source entrega um backing estável com bytes e Close. A abertura transfere esse owner ao Graph; falha de validação fecha-o. O Graph pode guardar uma função privada de release sem conhecer mmap ou filesystem. Chamadas Close sequenciais são idempotentes; depois da primeira, limpar as views internas. O chamador precisa encerrar uso antes de Close, inclusive iteradores e acesso a metadados de resultados.

Não criar finalizer como controle de recursos e não fabricar strings unsafe sobre o mapping. Bytes LE são acessados após validação sem copiar as grandes colunas, mas Go não impede SIGBUS se outro processo truncar o arquivo. Leitura concorrente é permitida; unmap concorrente não é.

Writer não materializa o snapshot inteiro em um `bytes.Buffer` para então escrever o arquivo em produção. Calcular diretório/tamanhos e emitir em sequência por buffers limitados. Sink de memória para testes pode acumular bytes. Seal local usa mapping/arquivo estável para validar, sem exigir duplicar o arquivo inteiro em heap.

Close da view de validação ocorre antes de Commit. Falha de Close/release antes de publicar é erro operacional; não escondê-la com defer cujo resultado seja ignorado. O sink faz Write com semântica `io.Writer`: o codec trata short write e propaga falha.

# Ingestão: Neptune CSV, ordenação e resiliência

## Entrada obrigatória

O chamador fornece **dois catálogos distintos**, nodes e edges. No driver local, são dois diretórios existentes e enumeráveis. `--nodes` e `--edges` são obrigatórios mesmo quando um deles está vazio. A responsabilidade de colocar entradas adequadas nas pastas é do usuário; o motor valida e rejeita individualmente o que não corresponde ao contrato.

Enumerar somente entradas imediatas. Não recursar, não seguir symlinks, não escolher por extensão, não ignorar arquivos escondidos. Tentar o header de cada arquivo regular. Entradas não regulares são rejeitadas com diagnóstico. Não mover um arquivo edge encontrado no catálogo nodes para a segunda fase.

Enumerar os dois catálogos no início e guardar descriptors estáveis, não todo o conteúdo. Uma falha em enumerar um catálogo raiz é fatal: a operação não possui um universo de entrada conhecido. Falha ao abrir/ler **uma fonte individual** não derruba o lote.

## Pipeline

```text
validar opções e enumerar catálogos
  -> para cada fonte nodes: abrir, validar header esperado, ler contribuições
  -> consolidar todos os nodes (incluindo conflitos de propriedade)
  -> barreira: universo final de IDs de nodes
  -> para cada fonte edges: abrir, validar header esperado, resolver endpoints
  -> consolidar edges e quarentenar IDs estruturalmente inconsistentes
  -> canonicalizar -> CSR -> índices -> Graph
  -> serializar -> validar -> publicar
  -> relatório final
```

Ordenar descriptors por key em bytes torna os diagnósticos mais previsíveis, mas a semântica de merge não depende dessa ordenação. Antes da barreira, nenhum registro de edge é consumido. Não criar placeholder para node inexistente nem agendar retries esperando um node que já não pode surgir.

## Contrato de CSV do adapter Neptune

Alvo: **Gremlin CSV** (`format=csv`), não openCypher. As colunas reservadas, tipos e regras básicas vêm de [N1 e N2](REFERENCES.md). O restante desta seção é o perfil executável do nosso adapter; a política de resiliência/merge é própria do GopherGraph.

### Léxico

UTF-8 válido. Vírgula como separador; LF e CRLF como terminador. CR isolado fora de aspas é erro lexical. Campo entre aspas pode conter vírgula e quebras; `""` dentro dele representa uma aspa. Manter `was_quoted` e comprimento: campo ausente não é string vazia. Um registro lógico pode ocupar várias linhas físicas. Header é o registro lógico 1; o primeiro dado é 2.

Remover espaços ASCII fora das aspas nas bordas de campos não-quoted, como tolerância do dialeto Neptune; não remover espaços dentro de aspas nem interpretar escapes de linguagem de programação. Aceitar um BOM UTF-8 somente no início do stream. Não fazer reparo Unicode. NUL literal é rejeitado no registro; arquivos não UTF-8 não são reinterpretados em outro encoding.

Linha totalmente vazia fora de aspas é ignorada e não conta como registro. Um último registro bem formado pode terminar em EOF sem newline. Número diferente de campos do header rejeita somente o registro já delimitado. Campo quoted vazio é conteúdo; campo unquoted vazio é ausência.

### Headers

| Catálogo | Obrigatórios | Opcionais | Rejeições de arquivo |
|---|---|---|---|
| nodes | `~id` | `~label`, propriedades | `~from`, `~to`, coluna reservada desconhecida/repetida |
| edges | `~id`, `~from`, `~to` | `~label`, propriedades | obrigatório ausente, reservada desconhecida/repetida |

Header pode reordenar colunas. Uma propriedade é `nome:tipo`, `nome:tipo(single)`, `nome:tipo(set)`, `nome:tipo[]` ou `nome:tipo(set)[]`. O separador é um `:` não escapado; `\:` no nome significa dois-pontos literal. Nomes de propriedades não podem ficar vazios nem conter espaço, vírgula, CR ou LF. Rejeitar propriedades duplicadas no mesmo header após decodificar o nome. O mesmo nome pode aparecer em arquivos diferentes.

Tipos são case-insensitive; nomes de labels/keys/IDs são case-sensitive. `Boolean` é alias de `Bool`; as dez tags estão em DATA_MODEL.md. Cardinalidade default de node é set; de edge é single. Em edges, rejeitar header array ou cardinalidade set. `single[]` é header inválido. Um arquivo inválido por header é fechado, diagnosticado e não contribui com registros.

### IDs, labels e ausência

`~id`, `~from`, `~to` sem conteúdo são inválidos quando unquoted. `""` identifica a string vazia e é válida nesses campos; resolver endpoints vazios pelo mesmo dicionário de nodes.

Label de node é uma lista de nomes não vazios separados por `;`, deduplicada como conjunto. Edge recebe exatamente um label; não interpretar uma lista de labels de edge como múltiplas arestas. Ausência de coluna/valor de label aplica o default `vertex` ou `edge`; label explicitamente vazio (`""`) rejeita o registro. Segmento vazio na lista de labels também é inválido. O perfil não aceita `;` literal dentro de um label.

Propriedade ausente não adiciona valor nem conflito de cardinalidade. String quoted vazia adiciona `String("")`. Valor quoted vazio para tipo não textual rejeita o registro. Arrays dividem em `;`; em arrays String, `\;` é semicolon literal e os demais backslashes são preservados. Não tratar `\n`, `\t` ou `\u...` como escapes. Segmentos vazios em String[] são strings vazias; segmentos vazios em arrays numéricos são inválidos. Semicolons em String escalar são texto comum.

### Valores escalares

Inteiros: sinal opcional, dígitos decimais, consumo completo e faixa exata de Byte/Short/Int/Long; rejeitar lixo, hex e overflow. Bool aceita `true` e `false`; para corresponder à regra de coerção documentada do loader, outro token não vazio vira false **com warning `BOOL_COERCED`**, nunca silenciosamente. Não coagir número inválido a zero.

Float/Double: decimais e notação científica; aceitar `Infinity`, `+Infinity`, `-Infinity`, `NaN`; rejeitar `INF`, hex-float e sufixos. Após validar a gramática decimal do perfil, usar `strconv.ParseFloat` com bitSize 32/64; preservar o arredondamento da conversão e os bits resultantes. Não aceitar automaticamente todas as grafias que strconv aceita. Finito que transborda para infinito rejeita registro; underflow que arredonda para zero/subnormal é aceito. Preservar sinal do zero; canonicalizar NaN conforme DATA_MODEL.md. Para signed inteiros, usar `strconv.ParseInt` com base decimal e largura correta, depois da validação lexical. Nunca usar ParseBool como se implementasse a regra de coerção Neptune. [G9](REFERENCES.md)

Date/Datetime: guardar texto validado, sem cálculo de epoch. Aceitar `YYYY-MM-DD`, e data seguida de `T` e `HH:MM`, opcionalmente `:SS`; após segundos, aceitar fração de 1 a 9 dígitos. Com hora presente, aceitar sufixo ausente, `Z`, `+HH:MM`, `-HH:MM`, `+HHMM` ou `-HHMM`. Ano 0001..9999; calendário gregoriano, dias válidos e ano bissexto; hora 00..23, minuto/segundo 00..59; offset até 18:00, sem leap second. Não aceitar datas parciais, timezone nomeado ou normalizar texto. As formas adicionais de precisão/offset são contrato lexical local; semântica temporal e certificação de todas as variantes do serviço ficam fora.

Os exemplos do pacote cobrem as tags e fronteiras relevantes; formatos particulares de arquivos corporativos ainda não fornecidos não são declarados testados.

## Atomicidade do registro e conflitos de lote

Primeiro decodificar e validar o registro inteiro em staging. Se campo tipado, ID, label ou aridade for inválido, **nenhuma contribuição daquele registro entra no builder**. Isso impede que metade de uma linha rejeitada altere o grafo.

Após os registros individualmente válidos, consolidar por ID e chave:

| Situação | Resultado determinístico |
|---|---|
| Node repetido com labels diferentes | União dos labels e contribuições |
| `set` repetido | União de valores tipados sem duplicatas |
| `single` repetido com valor idêntico | Um valor; contribuição idempotente |
| `single` com valores diferentes | Remover todos os valores dessa chave no dono; diagnóstico de conflito |
| Declarações single/set incompatíveis, com contribuições presentes | Remover a propriedade inteira; diagnóstico |
| Edge repetida, mesmos endpoints e label | União de contribuições; propriedades de edge são single |
| Mesmo EdgeID externo com endpoints ou label diferentes | Quarentenar todo esse ID; nenhuma variante é publicada |
| EdgeIDs diferentes com a mesma tripla | Preservar ambas, inclusive propriedades |
| Edge com endpoint ausente | Rejeitar o registro; não participar do merge |

Não existe first-wins ou last-wins. A propriedade removida não reaparece ao chegar outra contribuição. Em conflito estrutural de edge, tombstone do ID impede sua ressurreição. Um registro com endpoint ausente não torna inválida uma edge válida de mesmo ID: ele já foi rejeitado antes de participar.

Conflitos eliminam grupos, não o lote. Guardar identidade do grupo, número de contribuições e pelo menos duas origens de diagnóstico determinísticas quando houver dois valores/estruturas conflitantes. Não guardar todos os CSVs em memória só para logging.

## Recuperação lexical e limites

Em um registro cujo fim é conhecido (aridade, tipo, UTF-8 em um campo delimitado), rejeitar e continuar no próximo registro lógico. Quando aspas/quebras deixam impossível reconhecer com confiança a próxima fronteira, emitir `CSV_SOURCE_UNRECOVERABLE`, preservar registros completos anteriores e encerrar **somente essa fonte**. Prosseguir para as demais.

Não usar “pular até o próximo newline” como recuperação universal: ele pode estar dentro de um campo válido. Aspas abertas até EOF não autorizam inventar linhas posteriores. Não prometer recuperar bytes estruturalmente ambíguos.

Limite default por registro lógico: **64 MiB**, configurável via `--max-record-bytes`, mínimo 1 KiB e máximo representável pela plataforma. Header conta para o limite. Até 65.536 colunas. O parser não precisa guardar bytes de um registro excessivo: pode drenar com estado lexical; se reencontrar um fim confiável, rejeita o registro e continua; se não, encerra a fonte. No header excessivo, rejeita o arquivo. Limite global de memória/IDs é uma condição de recursos, não truncamento silencioso.

## Diagnósticos e relatório

Evento estruturado: severidade, código estável, papel nodes/edges, source key, registro lógico/linha inicial quando conhecidos, entity ID quando conhecido, coluna e explicação curta. Eventos usam strings Go e campos tipados; o sink não pode reter slices temporários do decoder. Não imprimir linha completa nem valores de propriedades sensíveis por padrão. Escapar controle na saída local.

Emitir um evento por rejeição de registro/arquivo e um por grupo conflitante; warnings de coerção são distintos de perda. Erro do sink de diagnóstico é fatal antes da publicação, pois continuar sem reportar perdas viola o contrato.

Por catálogo, contar arquivos vistos, concluídos, rejeitados por header, falhas de I/O, fontes interrompidas por estrutura lexical e entradas não regulares. Essas categorias finais são disjuntas: erro de leitura é `sources_io_failed`, não também `sources_interrupted`; esta última fica para cauda CSV irrecuperável/limite sem recuperação. `sources_seen` inclui entradas não regulares. Em carga terminada sem falha global, seen é a soma das categorias finais. Contar registros completos vistos, rejeitados e staged. `staged = vistos - rejeitados`; **staged não significa entidade final**, pois duplicatas e conflitos são consolidados depois. Guardar ainda grupos de propriedades removidos, IDs de edge quarentenados e números finais de nodes/edges. `Report.Nodes` e `Report.Edges` são contagens do Graph consolidado; o resultado de publicação é separado e informa se o Graph chegou a ser persistido/publicado.

Trecho incompleto não é um registro completo e não entra em `records_seen`; o relatório indica cauda não recuperável sem inventar quantas linhas lógicas foram perdidas.

`load_completeness = COMPLETE` ou `PARTIAL`. Rejeição de dados/fonte/grupo torna parcial. Warning sem perda não torna parcial. Um catálogo vazio ou arquivos só de header podem gerar grafo vazio completo; todos os registros rejeitados geram grafo vazio parcial. Emitir `EMPTY_GRAPH`, mas não abortar apenas por não haver entidades válidas.

## Erros que realmente encerram o build

Argumentos ou catálogo raiz inválidos; cancelamento; limites de recursos declarados ou estouro de capacidade estrutural; invariante interna violada; falha de diagnóstico; falha ao serializar/validar/publicar o snapshot. Falha de abrir/ler arquivo individual é recuperável e conserva seu prefixo já admitido. A distinção entre essas situações é implementada em status, não por substring da mensagem.

A publicação é assunto de SNAPSHOT.md. Não declarar o snapshot anterior preservado quando o rename já ocorreu e falhou apenas a sincronização final do diretório.

## Implementação com encoding/csv

Não é correto traduzir cada campo devolvido por `csv.Reader.Read()` diretamente para presença/ausência: `,,` e `,"",` produzem a mesma string vazia na stdlib. Ela também normaliza CRLF no conteúdo de campos multiline. [G1](REFERENCES.md)

O adapter deve manter metadados lexicais de cada célula: `Present`, `Quoted`, conteúdo e origem. Uma camada pequena de framing preserva registro bruto/faixas dos campos, aplica o limite e identifica fim confiável; `encoding/csv` faz a validação/decodificação CSV onde possível. Complementar ausência versus vazio, espaços externos e preservação de CRLF dentro de texto a partir das faixas brutas. Não normalizar conteúdo do usuário silenciosamente. Este complemento é específico do dialeto, não uma biblioteca CSV genérica nova.

Não usar `ReadAll`, `strings.Split`, `bufio.Scanner` default ou `LazyQuotes=true`. Aridade errada em registro completo é recuperável. Um erro de aspas sem ressincronização comprovada encerra a fonte. `ReuseRecord` só pode ser usado se o builder copiar as contribuições que retém. Header e records devem ter limites antes de alocação descontrolada; limitar o leitor total do arquivo não equivale a limite por registro.

O reader pode ler por chunks de 1, 2, 7 ou 4096 bytes sem mudar semântica. `io.Reader` pode devolver `n>0` junto com `io.EOF` ou outro erro: consumir somente registros completos desses bytes antes de encerrar a fonte. Não perder o último registro válido sem newline. Não assumir que uma chamada read corresponde a uma linha.

## Relatório em Go

`Build` retorna `(*graph.Graph, Report, error)`. Bad data isolado retorna Graph válido, `Report.Completeness=Partial` e `error=nil`; erro operacional retorna nenhum Graph publicável, relatório parcial de andamento e erro tipado. O driver publica apenas quando Build terminou operacionalmente.

Estados finais das fontes permanecem disjuntos. Diagnóstico de rejeição é emitido uma vez pela orquestração; warnings do decoder não contam como perda. O sink retorna erro caso não consiga registrar os eventos: continuar silenciosamente não cumpre a carga resiliente auditável.

OOM fatal do runtime Go não é recuperado por `recover` nem convertido em "registro rejeitado". Evitar crescimento ilimitado com caps de registro, checagens de tamanho e medição de RSS. Uma fonte inacessível é recuperável; catálogo raiz não enumerável é fatal.

# Desempenho e orçamento de memória

## O que queremos provar

Representação compacta, travessias sem enumeração de caminhos e custo previsível. O pacote não inventa SLA ou promete velocidade superior a um banco sem medir o corpus relevante.

## Modelo de espaço do snapshot

Sem padding, header/diretório e conteúdo dos índices:

- Strings: `8*(S+1) + B` bytes.
- Dados de nodes: `4*N + 16*(N+1) + 4*L + 16*Pn`.
- Dados de edges: `16*E + 8*(E+1) + 16*Pe`.
- CSR nos dois sentidos: `16*(N+1) + 16*E`.
- Índices: `24*Glabel + 4*(L+E) + 4*K + 32*Gprop + 4*Qprop`.

Somar header 64, diretório 768 e padding menor que 64 por intervalo de seção. O cálculo deve bater com `file_size`, não com uma estimativa baseada em objetos em heap. Repetir endpoints nos CSR e na tabela de edges é custo escolhido e mensurável.

## Tempo e scratch

BFS expande cada node alcançado uma vez e examina sua adjacência uma vez. Custo de expansão `O(Vr+Er)`. Inicializar bitmap denso custa `O(N/64)`; não omitir isso ao descrever uma consulta pequena em grafo grande. A fila ocupa no máximo O(N) IDs. Não há recursão proporcional ao tamanho do grafo.

Subgrafo acrescenta bitmap de edges, com custo de inicialização `O(E/64)`, e seleção das edges entre nodes alcançados. `between` usa dois alcances, interseção e seleção; não tem custo proporcional ao número de caminhos possíveis.

Builder pode usar memória maior que o snapshot, por staging, mapas e ordenação; medir pico e evitar múltiplas cópias desnecessárias. Esse build offline não é licença para alocação descontrolada. Tratar falha de recurso explicitamente, sem cortar os dados silenciosamente.

## Medição obrigatória em B11

Usar três escalas sintéticas sugeridas: 1 mil nodes/5 mil edges, 10 mil/50 mil, 100 mil/500 mil; reduzir a última somente por limitação real registrada da máquina, sem fingir teste de escala maior. Misturar grafo linear, diamantes encadeados, ciclos, hub, componentes desconexas, labels e propriedades.

Registrar separadamente: ingestão+merge, canonicalização/CSR/índices, escrita+validação+commit, abrir+validar snapshot, alcance, subgrafo e DOT. Medir pico de RSS do processo de build e de consulta e tamanho do snapshot. Usar relógio monotônico e pelo menos cinco consultas sobre Graph já aberto, reportando mediana e dispersão simples.

Distinguir execução com cache aquecido de abertura inicial. Não exigir limpeza do page cache com privilégios; não afirmar que uma execução é cold-cache sem controle. Informar opções de execução e arquitetura e tamanho do dataset, sem pins de toolchain ou hashes de commit.

## Gate objetivo

As respostas coincidem com o oráculo; contadores de instrumentação de teste mostram no máximo uma expansão por node e uma inspeção por ocorrência de adjacência examinada; diamantes não produzem listas de caminhos. O arquivo respeita a fórmula. Não existem vazamentos ou crescimento residual em repetidas consultas/open-close.

Uma latência surpreendente exige localizar qual fase custa mais; não autoriza adicionar planner, cache, threads ou sistema distribuído sem demanda. O gráfico grande real do usuário, quando disponível, complementa a qualificação; não bloqueia o desenvolvimento sintético inicial.

## Medições específicas de Go

Usar benchmarks de `testing.B` com `ReportAllocs`. Perfil CPU/heap via tooling padrão/pprof quando necessário. Medir query em Graph já aberto e medir abertura/validação separadamente. O número de alocações não pode crescer com cada edge examinada no hot loop. Maps/strings de staging não devem sobreviver sem motivo depois de consolidar/liberar o builder.

O mapping está fora das grandes estruturas heap usuais; heap baixo não é RAM do processo baixa. Registrar bytes mapeados, heap/allocs, RSS e pico do builder. GC pode reter capacidade para reuso, portanto não exigir RSS igual após cada GC/Close; exigir mappings/descritores liberados e ausência de crescimento residual sem limite em repetição.

Não desativar GC para maquiar benchmark. Não adicionar `sync.Pool`, concorrência interna, unsafe ou cache para fechar uma meta inexistente. Se encoding/binary aparecer como custo relevante, registrar evidência e otimizar localmente dentro do contrato seguro; uma alternativa unsafe é trabalho futuro deliberado, não requisito de aceite.

Não se presume que Go iguala/supera C ou Java. A referência de sucesso é correção e custo observado na arquitetura escolhida, sem overhead de engine de queries genérica.

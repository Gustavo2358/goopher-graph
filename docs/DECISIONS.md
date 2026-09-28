# Decisões e justificativas

Este registro substitui uma coleção de ADRs repetitivos. Cada entrada fixa a escolha, o motivo e a consequência. Não há versão de produto prévia nem protocolo de aprovação por decisão.

| ID | Decisão | Por quê / consequência |
|---|---|---|
| D01 | Go em builder, engine, CLI e integrações futuras | Uma linguagem suportada no ambiente; stdlib reduz código incidental. Não depende de vantagem de performance presumida. |
| D02 | Stdlib + x/sys/unix isolado; sem cgo de produto | Evita duas toolchains e bindings. Race detector é ferramenta de teste, com requisitos próprios. |
| D03 | Vertical slices e interfaces locais às fronteiras | Coesão por capacidade sem misturar core com filesystem/CLI. Sem Clean Architecture cerimonial. |
| D04 | Directed property multigraph | Preserva IDs e propriedades de edges, paralelas, loops e multilabel de nodes. Não deduplicar por tripla. |
| D05 | Catálogos nodes/edges explícitos | Usuário declara papéis; o motor garante nodes primeiro. Arquivo no lugar errado não é movido por inferência. |
| D06 | Neptune Gremlin CSV | Reuso dos mesmos arquivos. Sem serviço Neptune, Gremlin, S3 ou equivalência transacional. |
| D07 | Resiliência por registro/fonte | Dados ruins não abortam lote. Rejeições são diagnosticadas; grafo parcial continua coerente. |
| D08 | Merge independente da ordem | União de set; conflito single remove a propriedade; conflito estrutural de EdgeID quarentena o ID. Sem first/last-wins. |
| D09 | Canonicalização final e IDs uint32 | Snapshot determinístico. Offsets/contagens uint64. IDs internos só valem no Graph de origem. |
| D10 | CSR forward e reverse | Território/antiterritório não precisam reconstruir índice. Duplicação de adjacência é custo explícito. |
| D11 | Postings persistentes e bitsets de trabalho | Não há bitmap gigante para cada valor de propriedade. Índices de propriedades são opt-in. |
| D12 | Snapshot imutável com layout explícito | mapeável e testável por leitor independente. Não usar gob/JSON como persistência do runtime. |
| D13 | Código próprio seguro primeiro | Bytes LE via encoding/binary; sem unsafe inicial. Ganhos supostos não justificam risco sem perfil. |
| D14 | Validação integral na abertura | Não expor offsets corrompidos. Custo de abertura é medido separadamente; não chamar mmap de O(1). |
| D15 | Staging + validação + rename + sync | Erro antes da publicação preserva anterior. Erro de durabilidade após rename é reportado sem falso rollback. |
| D16 | Funções/programas Go para consultas | API baixa e primitivas úteis bastam. Nenhuma DSL, planner, plugin registry ou executor universal. |
| D17 | Alcance reflexivo e subgrafo completo | API inclui origem; listagem pode omitir. Preserva convergências/ciclos/paralelas, não só árvore BFS. |
| D18 | Between é região de alcançabilidade | Interseção forward/reverse; não enumera caminhos simples nem promete sua união exata em ciclos. |
| D19 | Graph imutável, workspace por query | Permite leituras concorrentes. Chamador coordena Close; sem pool global ou paralelismo interno obrigatório. |
| D20 | CLI agora, cloud depois | Ports reais permitem adapters S3/HTTP futuros, mas não os implementamos antecipadamente. |
| D21 | Lean SDD | Prompt curto, backlog testável, um arquivo de progresso. go.mod/go.sum normais; sem pins de spec, SHAs ou cerimônia. |

## Trade-offs aceitos

O builder pode usar mais RAM que o snapshot e ainda é in-memory: external sorting não está incluído. A validação integral pode aquecer páginas antes da primeira consulta. Strings devolvidas pela API podem alocar cópias para segurança de lifetime. Datas são texto tipado, não instantes normalizados. Carga parcial registra perda; não repara os dados do usuário.

Go elimina trabalho manual de coleções/memória heap, mas não elimina contratos entre formatos, races, overflow de tamanhos ou lifecycle de mmap. As restrições são verificadas em testes, não justificadas por popularidade de linguagem.

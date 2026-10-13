#!/usr/bin/env bash
# validate-agents.sh — Audita conformidade de agentes com padrão
# Uso: bash .claude/scripts/validate-agents.sh [agent-name]

AGENTS_DIR=".claude/agents"

validate_agent() {
    local agent_dir="$1"
    local agent_name="$2"
    local agent_md="$agent_dir/AGENT.md"

    if [ ! -f "$agent_md" ]; then
        return
    fi

    echo "📋 $agent_name"

    local errors=0
    local warnings=0

    # Frontmatter
    if ! head -1 "$agent_md" | grep -q "^---$"; then
        echo "  ❌ Sem frontmatter"
        ((errors++))
    fi

    if ! grep -q "^name:" "$agent_md"; then
        echo "  ❌ Falta 'name'"
        ((errors++))
    fi

    if ! grep -q "^description:" "$agent_md"; then
        echo "  ❌ Falta 'description'"
        ((errors++))
    fi

    if ! grep -q "^model:" "$agent_md"; then
        echo "  ⚠️  Sem 'model'"
        ((warnings++))
    fi

    if ! grep -q "^allowed-tools:" "$agent_md"; then
        echo "  ⚠️  Sem 'allowed-tools'"
        ((warnings++))
    fi

    # Estrutura
    local h1=$(grep -c "^# " "$agent_md" || true)
    if [ "$h1" -ne 1 ]; then
        echo "  ❌ H1 count: $h1 (deve ser 1)"
        ((errors++))
    fi

    # H2 com emoji
    local h2_count=$(grep -c "^## " "$agent_md" || true)
    if [ "$h2_count" -gt 0 ]; then
        local h2_emoji=$(grep "^## " "$agent_md" | head -1)
        if echo "$h2_emoji" | grep -qE "^## [^a-zA-Z]"; then
            echo "  ✅ H2 com emoji ($h2_count seções)"
        else
            echo "  ⚠️  H2 sem emoji no início"
            ((warnings++))
        fi
    fi

    # Anti-patterns
    if grep -q "This agent" "$agent_md"; then
        echo "  ❌ Contém 'This agent'"
        ((errors++))
    fi

    if grep -q "Use this to" "$agent_md"; then
        echo "  ❌ Contém 'Use this to'"
        ((errors++))
    fi

    # Tamanho
    local lines=$(wc -l < "$agent_md")
    if [ "$lines" -gt 300 ]; then
        echo "  ⚠️  Grande ($lines linhas, recomendado <300)"
        ((warnings++))
    fi

    # Seções esperadas
    local has_responsabilidade=$(grep -q "Responsabilidade\|responsabilidade" "$agent_md" && echo "yes" || echo "no")
    local has_protocolo=$(grep -q "Protocolo\|protocolo\|Executar" "$agent_md" && echo "yes" || echo "no")
    local has_verificacoes=$(grep -q "Verificacoes\|verificações\|Checklist" "$agent_md" && echo "yes" || echo "no")
    local has_formato=$(grep -q "Formato\|formato\|saída\|Output" "$agent_md" && echo "yes" || echo "no")
    local has_restricoes=$(grep -q "Restricoes\|Restrições" "$agent_md" && echo "yes" || echo "no")
    local has_contexto=$(grep -q "Contexto\|contexto\|Stack\|stack" "$agent_md" && echo "yes" || echo "no")

    if [ "$has_responsabilidade" = "no" ]; then
        echo "  ⚠️  Sem seção 'Responsabilidade'"
        ((warnings++))
    fi

    if [ "$has_protocolo" = "no" ]; then
        echo "  ⚠️  Sem seção 'Protocolo'"
        ((warnings++))
    fi

    if [ "$has_formato" = "no" ]; then
        echo "  ⚠️  Sem seção 'Formato de saída'"
        ((warnings++))
    fi

    if [ "$has_restricoes" = "no" ]; then
        echo "  ⚠️  Sem seção 'Restrições'"
        ((warnings++))
    fi

    if [ "$errors" -eq 0 ]; then
        if [ "$warnings" -eq 0 ]; then
            echo "  ✅ OK"
        else
            echo "  ⚠️  OK com avisos"
        fi
    fi
    echo ""
}

main() {
    local target="${1:-.}"

    if [ "$target" != "." ] && [ -d "$AGENTS_DIR/$target" ]; then
        echo "Validando agente: $target"
        echo ""
        validate_agent "$AGENTS_DIR/$target" "$target"
    else
        echo "🔍 Validando todos os agentes..."
        echo ""

        cd "$AGENTS_DIR"
        for dir in */; do
            agent_name="${dir%/}"
            validate_agent "$agent_name" "$agent_name"
        done
        cd - > /dev/null
    fi
}

main "$@"

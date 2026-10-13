#!/usr/bin/env bash
# validate-skills.sh — Audita conformidade de skills com padrão
# Uso: bash .claude/scripts/validate-skills.sh [skill-name]

SKILLS_DIR=".claude/skills"

validate_skill() {
    local skill_dir="$1"
    local skill_name="$2"
    local skill_md="$skill_dir/SKILL.md"

    if [ ! -f "$skill_md" ]; then
        return
    fi

    echo "📋 $skill_name"

    local errors=0
    local warnings=0

    # Frontmatter
    if ! head -1 "$skill_md" | grep -q "^---$"; then
        echo "  ❌ Sem frontmatter"
        ((errors++))
    fi

    if ! grep -q "^name:" "$skill_md"; then
        echo "  ❌ Falta 'name'"
        ((errors++))
    fi

    if ! grep -q "^description:" "$skill_md"; then
        echo "  ❌ Falta 'description'"
        ((errors++))
    fi

    # Estrutura
    local h1=$(grep -c "^# " "$skill_md" || true)
    if [ "$h1" -ne 1 ]; then
        echo "  ❌ H1 count: $h1 (deve ser 1)"
        ((errors++))
    fi

    # Anti-patterns
    if grep -q "This skill" "$skill_md"; then
        echo "  ❌ Contém 'This skill'"
        ((errors++))
    fi

    if grep -q "Use this to" "$skill_md"; then
        echo "  ❌ Contém 'Use this to'"
        ((errors++))
    fi

    # Tamanho
    local lines=$(wc -l < "$skill_md")
    if [ "$lines" -gt 500 ]; then
        echo "  ⚠️  Grande ($lines linhas)"
        ((warnings++))
    fi

    if [ "$errors" -eq 0 ]; then
        echo "  ✅ OK"
    fi
    echo ""
}

main() {
    local target="${1:-.}"

    if [ "$target" != "." ] && [ -d "$SKILLS_DIR/$target" ]; then
        echo "Validando: $target"
        echo ""
        validate_skill "$SKILLS_DIR/$target" "$target"
    else
        echo "🔍 Validando todas as skills..."
        echo ""

        cd "$SKILLS_DIR"
        for dir in */; do
            skill_name="${dir%/}"
            validate_skill "$skill_name" "$skill_name"
        done
        cd - > /dev/null
    fi
}

main "$@"

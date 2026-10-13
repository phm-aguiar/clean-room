#!/usr/bin/env bash
# validate-docs-markdown.sh — Verifica regras de estilo Markdown (skill markdown-writing)
# Uso: bash .claude/scripts/validate-docs-markdown.sh [arquivo.md | diretório]
#      Padrão: ai-memory/docs (ignora assets/)
# Saída: exit 0 se nenhum ❌; exit 1 caso contrário. ⚠️ não reprova.
#
# Complementa validate-docs-clean-room.sh (que já cobre H1 único e links quebrados).
# Regras:
#   ❌ Imagem sem texto alternativo (alt vazio)
#   ⚠️  Cerca de código de abertura sem linguagem

TARGET="${1:-ai-memory/docs}"

total_errors=0
total_warnings=0
total_files=0
failed_files=0

check_file() {
    local file="$1"
    local errors=0 warnings=0 out=""

    # Cercas de abertura sem linguagem (fecha/abre pela contagem de ```)
    local fences
    fences=$(awk '/^ *```/{ if(!open){ if($0 ~ /^ *```[ \t]*$/) print NR; open=1 } else open=0 }' "$file")
    if [ -n "$fences" ]; then
        local n
        n=$(printf '%s\n' "$fences" | grep -c .)
        out+="  ⚠️  Cerca de código sem linguagem ($n) — linhas: $(printf '%s' "$fences" | head -5 | paste -sd, -)"$'\n'
        ((warnings++))
    fi

    # Imagens sem alt text
    local img
    img=$(grep -nE '!\[\]\(' "$file" | head -5)
    if [ -n "$img" ]; then
        out+="  ❌ Imagem sem texto alternativo"$'\n'"$(echo "$img" | cut -c1-140 | sed 's/^/       /')"$'\n'
        ((errors++))
    fi

    echo "📋 $file"
    if [ "$errors" -eq 0 ] && [ "$warnings" -eq 0 ]; then
        echo "  ✅ OK"
    else
        printf '%s' "$out"
    fi
    echo ""

    ((total_files++))
    [ "$errors" -gt 0 ] && ((failed_files++))
    total_errors=$((total_errors + errors))
    total_warnings=$((total_warnings + warnings))
}

if [ -f "$TARGET" ]; then
    check_file "$TARGET"
elif [ -d "$TARGET" ]; then
    echo "🔍 Validando estilo Markdown em $TARGET (ignora assets/)..."
    echo ""
    while IFS= read -r f; do
        check_file "$f"
    done < <(find "$TARGET" -name '*.md' -not -path '*/assets/*' | sort)
else
    echo "❌ Alvo não encontrado: $TARGET"
    exit 2
fi

echo "────────────────────────────────────────"
echo "Arquivos: $total_files | Reprovados: $failed_files | ❌ $total_errors | ⚠️  $total_warnings"
[ "$total_errors" -eq 0 ]

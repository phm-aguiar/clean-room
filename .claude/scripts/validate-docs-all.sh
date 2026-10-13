#!/usr/bin/env bash
# validate-docs-all.sh — Roda todas as validações de ai-memory/docs em paralelo
# Uso: bash .claude/scripts/validate-docs-all.sh [arquivo.md | diretório]
#      Padrão: ai-memory/docs
#      REPORT_DIR (padrão: ai-memory/.validation) recebe um relatório por validador.
# Saída: exit 0 se nenhum validador reprovou; exit 1 caso contrário.

TARGET="${1:-ai-memory/docs}"
REPORT_DIR="${REPORT_DIR:-ai-memory/.validation}"
SCRIPTS=".claude/scripts"

mkdir -p "$REPORT_DIR" || exit 2

declare -A NAMES=(
    [clean-room]="validate-docs-clean-room.sh"
    [markdown]="validate-docs-markdown.sh"
)

pids=()
for key in "${!NAMES[@]}"; do
    bash "$SCRIPTS/${NAMES[$key]}" "$TARGET" > "$REPORT_DIR/$key.txt" 2>&1 &
    pids+=("$!:$key")
done

status=0
for entry in "${pids[@]}"; do
    pid="${entry%%:*}"
    key="${entry#*:}"
    wait "$pid"
    rc=$?
    summary=$(tail -1 "$REPORT_DIR/$key.txt")
    if [ "$rc" -eq 0 ]; then
        echo "✅ $key — $summary"
    else
        echo "❌ $key — $summary"
        status=1
    fi
done

echo ""
echo "Relatórios completos em $REPORT_DIR/ (clean-room.txt, markdown.txt)"
exit "$status"

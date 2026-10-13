#!/usr/bin/env bash
# validate-docs-clean-room.sh — Audita autossuficiência e neutralidade de linguagem da documentação
# Uso: bash .claude/scripts/validate-docs-clean-room.sh [arquivo.md | diretório]
#      Padrão: ai-memory/docs (conteúdo: ignora assets/)
#      Nomes README*/index* são reprovados em todo ai-memory/, inclusive assets/.
#      AI_MEMORY_ROOT (padrão: ai-memory) define a raiz varrida para essa regra.
# Saída: exit 0 se nenhum ❌; exit 1 caso contrário. ⚠️ não reprova.
#
# Distinção importante:
#   - NOME DE COMPONENTE lógico (ex.: ai-memory-core, ai-memory-store) é permitido.
#   - CAMINHO DE CÓDIGO (akita/, crates/x, src/x.rs) e idioma Rust são proibidos.

TARGET="${1:-ai-memory/docs}"

# ❌ Referências a código/repositório externo
RE_EXTERNAL='(akita/|(^|[^a-zA-Z-])crates/[a-z]|companions/[a-z]|[A-Za-z0-9_/-]+\.rs\b|Cargo\.(toml|lock)|rust-toolchain|\bsrc/[a-z_]+/)'
# ❌ Idioma/bibliotecas Rust
RE_RUSTISM='(\b(rusqlite|tokio|serde|serde_json|axum|clap|anyhow|thiserror|reqwest|rmcp|tracing)\b|\b(Arc|Mutex|RwLock|Option|Result|Vec|Box)<|\bimpl([ <])[A-Z]|\b(pub )?(async )?fn [a-z_]+[(<]|#\[(derive|cfg|tokio|test)|\bstruct [A-Z][A-Za-z]*|\btrait [A-Z][A-Za-z]*|&mut |\.unwrap\(\)|\.expect\(")'
# ❌ Caminhos de diretórios de código/repo (rotas /internal/... e /usr/lib/... são permitidas)
RE_REPO_DIRS='(^|[^A-Za-z0-9_.~-])(src|crates|cmd|pkg|akita|scripts|frontend|specs|internal-pkg)/[A-Za-z0-9_.-]'
# ❌ Nomes de linguagem, gerenciador de pacotes e biblioteca (spec F001: "sem linguagem nem biblioteca")
RE_LANG_NAMES='\b([Rr]ust|[Tt]ype[Ss]cript|JavaScript|[Gg]olang|[Pp]ython|cargo (build|test|run|fmt|clippy|deny|install|check)|npm|npx|Node\.js|pnpm|yarn|tokio|serde|rusqlite|pgx|mcp-remote)\b'
# ❌ Organização, autor ou diretório do repositório de origem (spec F001: sem referências a ele)
RE_ORIGIN='(akitaonrails|akita/|Fabio Akita|github\.com/akita)'
# ❌ Remissões ao código-fonte
RE_SEECODE='([Vv]eja o c[óo]digo|[Vv]er o c[óo]digo|c[óo]digo-fonte|see the (source|code)|read the (source|code)|consult the source)'
# ⚠️ Pendências e comandos de build do código original
RE_PENDING='(\bTBD\b|\bTODO\b|\bFIXME\b|a confirmar|a definir)'
RE_BUILD='(\bcargo (build|run|test|install|clippy|fmt|deny)\b)'
# ❌ Nomes de arquivo proibidos em ai-memory/ (README* e index*, qualquer caixa)
AI_MEMORY_ROOT="${AI_MEMORY_ROOT:-ai-memory}"

total_errors=0
total_warnings=0
total_files=0
failed_files=0

# Remove blocos de código? Não: Rust-isms em exemplos também contam. Mantém o arquivo inteiro.
check_pattern() {
    local file="$1" label="$2" regex="$3" severity="$4"
    local hits
    hits=$(grep -nE "$regex" "$file" | head -5)
    if [ -n "$hits" ]; then
        local count
        count=$(grep -cE "$regex" "$file")
        if [ "$severity" = "error" ]; then
            echo "  ❌ $label ($count)"
        else
            echo "  ⚠️  $label ($count)"
        fi
        echo "$hits" | cut -c1-140 | sed 's/^/       /'
        [ "$severity" = "error" ] && return 1
        return 2
    fi
    return 0
}

check_name() {
    local base lower
    base=$(basename "$1")
    lower=$(printf '%s' "$base" | tr '[:upper:]' '[:lower:]')
    case "$lower" in
        readme*|index*)
            echo "  ❌ Nome de arquivo proibido (README/index): $base"
            return 1 ;;
    esac
    return 0
}

# Varre assets/ (ignorado pelo conteúdo) em busca de README*/index*
check_name_tree() {
    local f
    while IFS= read -r f; do
        echo "📋 $f"
        echo "  ❌ Nome de arquivo proibido (README/index) em assets/"
        echo ""
        ((total_errors++))
        ((failed_files++))
        ((total_files++))
    done < <(find "$AI_MEMORY_ROOT" -type f -path '*/assets/*' \( -iname 'README*' -o -iname 'index*' \) | sort)
}

validate_doc() {
    local file="$1"
    local errors=0 warnings=0

    echo "📋 $file"

    check_name "$file" || ((errors++))
    check_pattern "$file" "Referência a código/repo externo" "$RE_EXTERNAL" error || ((errors++))
    check_pattern "$file" "Idioma/biblioteca Rust" "$RE_RUSTISM" error || ((errors++))
    check_pattern "$file" "Caminho de diretório de código/repo" "$RE_REPO_DIRS" error || ((errors++))
    check_pattern "$file" "Nome de linguagem/biblioteca/gerenciador" "$RE_LANG_NAMES" error || ((errors++))
    check_pattern "$file" "Remissão ao código-fonte" "$RE_SEECODE" error || ((errors++))
    check_pattern "$file" "Nome da organização/repo de origem" "$RE_ORIGIN" error || ((errors++))

    check_pattern "$file" "Pendência (TBD/TODO)" "$RE_PENDING" warn
    [ $? -eq 2 ] && ((warnings++))
    check_pattern "$file" "Comando de build do código original" "$RE_BUILD" warn
    [ $? -eq 2 ] && ((warnings++))

    # Exatamente um H1 fora de blocos de código
    local h1
    h1=$(awk '/^```/{f=!f; next} !f && /^# /{n++} END{print n+0}' "$file")
    if [ "$h1" -ne 1 ]; then
        echo "  ❌ H1 count: $h1 (deve ser 1)"
        ((errors++))
    fi

    # Links relativos para .md inexistentes
    local dir link target broken=0
    dir=$(dirname "$file")
    while IFS= read -r link; do
        target="${link%%#*}"
        [ -z "$target" ] && continue
        if [ ! -e "$dir/$target" ]; then
            echo "  ❌ Link quebrado: $link"
            ((broken++))
        fi
    done < <(grep -oE '\]\([^)#:]+\.md(#[^)]*)?\)' "$file" | sed -E 's/^\]\(//; s/\)$//' | sort -u)
    errors=$((errors + broken))

    local lines
    lines=$(wc -l < "$file")
    if [ "$lines" -gt 800 ]; then
        echo "  ⚠️  Grande ($lines linhas) — considere dividir"
        ((warnings++))
    fi

    if [ "$errors" -eq 0 ]; then
        echo "  ✅ OK"
    else
        ((failed_files++))
    fi
    total_errors=$((total_errors + errors))
    total_warnings=$((total_warnings + warnings))
    ((total_files++))
    echo ""
}

main() {
    if [ -f "$TARGET" ]; then
        validate_doc "$TARGET"
    elif [ -d "$TARGET" ]; then
        echo "🔍 Validando clean-room em $TARGET (ignora assets/)..."
        echo ""
        while IFS= read -r f; do
            validate_doc "$f"
        done < <(find "$TARGET" -name '*.md' -not -path '*/assets/*' | sort)
        check_name_tree
    else
        echo "❌ Alvo não encontrado: $TARGET"
        exit 2
    fi

    echo "────────────────────────────────────────"
    echo "Arquivos: $total_files | Reprovados: $failed_files | ❌ $total_errors | ⚠️  $total_warnings"
    [ "$total_errors" -eq 0 ]
}

main

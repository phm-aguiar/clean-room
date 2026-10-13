#!/usr/bin/env bash
# check-sdd.sh — Valida a estrutura SDD do repositório

set -e

PASS=0
FAIL=0
WARN=0

check() {
  local path="$1"
  local label="$2"
  local required="${3:-true}"

  if [ -e "$path" ]; then
    if [ -f "$path" ] && [ ! -s "$path" ]; then
      echo "  WARN  $label (vazio)"
      WARN=$((WARN + 1))
    else
      echo "  PASS  $label"
      PASS=$((PASS + 1))
    fi
  else
    if [ "$required" = "true" ]; then
      echo "  FAIL  $label (ausente)"
      FAIL=$((FAIL + 1))
    else
      echo "  WARN  $label (ausente — opcional)"
      WARN=$((WARN + 1))
    fi
  fi
}

echo "SDD Structure Check"
echo "==================="

echo ""
echo ".github/memory/"
check ".github/memory" "  directory"
check ".github/memory/constitution.md" "  constitution.md"
check ".github/memory/tech.md" "  tech.md"

echo ""
echo "specs/"
check "specs" "  directory"
check "specs/domain-events" "  domain-events/"
check "specs/features" "  features/"
check "specs/infra" "  infra/"

echo ""
echo "features/"
if [ -d "specs/features" ]; then
  for feature_dir in specs/features/*/; do
    [ -d "$feature_dir" ] || continue
    feature_name=$(basename "$feature_dir")
    check "$feature_dir" "  $feature_name/"
    check "${feature_dir}spec.md" "    spec.md"
    check "${feature_dir}plan.md" "    plan.md"
    check "${feature_dir}tasks.md" "    tasks.md"
  done
fi

echo ""
echo "CLAUDE.md"
check "CLAUDE.md" "  file" "false"

echo ""
echo "==================="
echo "PASS: $PASS  FAIL: $FAIL  WARN: $WARN"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi

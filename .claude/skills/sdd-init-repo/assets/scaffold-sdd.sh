#!/usr/bin/env bash
# scaffold-sdd.sh — Cria a estrutura de diretórios SDD

set -e

echo "Inicializando estrutura SDD..."

# Nível 1: Memória de Contexto Global
mkdir -p .github/memory

# Nível 2: specs
mkdir -p specs/domain-events
mkdir -p specs/features
mkdir -p specs/infra

echo "Estrutura criada:"
echo "  .github/memory/"
echo "  specs/"
echo "    domain-events/"
echo "    features/"
echo "    infra/"

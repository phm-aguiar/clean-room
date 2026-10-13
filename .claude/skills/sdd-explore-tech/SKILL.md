---
name: sdd-explore-tech
description: Scan repository and populate .github/memory/tech.md with detected technology stack (languages, frameworks, build tools, CI, linters, test patterns).
when_to_use: After sdd-init-repo to populate tech.md, or anytime the user wants to map or update the project's technology stack.
argument-hint: "[--update]"
allowed-tools: Read Write Bash
---

# Explore Tech Stack

Scan repository manifest files and build a canonical tech.md profile.

## Quick Start

**Step 1: Detect manifests**

Scan for standard manifest files: `go.mod`, `package.json`, `Cargo.toml`, `pom.xml`, `Dockerfile`, etc. (see Reference Index for full list).

**Step 2: Extract versions and frameworks**

For each manifest found, read and extract: language version, primary frameworks, build tools.

**Step 3: Load template and populate**

Use `assets/tech-template.md`. Fill with detected data. Use `—` for unfound entries. If `tech.md` exists, ask: overwrite or merge?

---

## Reference Index

### Manifest Detection

| File | Language/Tool |
|---|---|
| `go.mod` | Go |
| `package.json` | Node.js/TypeScript |
| `Cargo.toml` | Rust |
| `pom.xml`, `build.gradle*` | Java/Kotlin |
| `requirements.txt`, `pyproject.toml`, `setup.py` | Python |
| `Gemfile` | Ruby |
| `mix.exs` | Elixir |
| `CMakeLists.txt` | C/C++ |
| `*.csproj`, `*.sln` | .NET |

### Auxiliary Detection

Scan also for:
- CI/CD: `.github/workflows/`, `.gitlab-ci.yml`, `Jenkinsfile`
- Linters: `.golangci.yml`, `.eslintrc*`, `pylintrc`
- Docker: `Dockerfile`, `docker-compose*`
- Task runners: `Makefile`, `justfile`
- Tests: `_test.go`, `*.test.ts`, `*.spec.ts`

---

## Best Practices Checklist

- [ ] `.github/memory/tech.md` exists with all template sections
- [ ] Each entry has source (detected file) or `—`
- [ ] No dependencies invented
- [ ] User can manually adjust if needed
- [ ] No hardcoded assumptions about project structure

---

## Additional Resources

- `assets/tech-template.md` — canonical tech.md template

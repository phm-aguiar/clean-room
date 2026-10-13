# Code Blocks

## Inline Code

Use backticks for code within prose:

```markdown
Run `npm install` to install dependencies.
The `NODE_ENV` variable controls the environment.
Use `git commit -m "message"` to save changes.
```

## Fenced Code Blocks

Use triple backticks with language identifier:

````markdown
```python
def hello(name):
    print(f"Hello, {name}!")
```

```bash
npm install
npm start
```

```javascript
const x = 5;
console.log(x);
```
````

## Language Identifiers

Common languages:

| Language | ID |
| --- | --- |
| Python | `python` |
| JavaScript | `javascript` or `js` |
| TypeScript | `typescript` or `ts` |
| Bash/Shell | `bash` or `sh` |
| JSON | `json` |
| YAML | `yaml` or `yml` |
| Markdown | `markdown` or `md` |
| HTML | `html` |
| CSS | `css` |
| SQL | `sql` |
| Ruby | `ruby` |
| Go | `go` |
| Rust | `rust` |
| Java | `java` |

## Code Block Options

### Syntax Highlighting

Always specify the language for proper highlighting:

```python
# Good — language specified
def calculate(x, y):
    return x + y
```

### Line Numbers (GitHub)

Some hosts support line number prefixes:

```python
1: def calculate(x, y):
2:     return x + y
```

### Comments for Context

Add comments at the start to provide context:

```bash
# Deploy to production
./scripts/deploy.sh --env production
```

## Indented Code Blocks

Indent by 4 spaces (not recommended — use fenced blocks instead):

```markdown
    const x = 5;
    console.log(x);
```

## Code Block Best Practices

- **Always specify language** — enables syntax highlighting
- **Keep focused** — show the relevant snippet, not the entire file
- **Add context comments** — especially for commands or configuration
- **Use `text` for plain output** — not empty ` ``` `
- **Break long lines** — for readability

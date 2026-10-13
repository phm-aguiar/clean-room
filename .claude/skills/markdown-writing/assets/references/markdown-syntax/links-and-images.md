# Links and Images

## Links

### Inline Links

```markdown
[Link text](https://example.com)
[Link with title](https://example.com "Hover title")
```

### Reference-style Links

```markdown
[Link text][reference]
[Link text][1]

[reference]: https://example.com
[1]: https://example.com
```

### Automatic Links

```markdown
<https://example.com>
<user@example.com>
```

### Bare URLs (GitHub Flavored Markdown)

```markdown
Visit https://example.com for more info.
```

## Images

### Inline Images

```markdown
![Alt text](image.png)
![Alt text](image.png "Image title")
![Alt text](path/to/image.png)
```

### Reference-style Images

```markdown
![Alt text][image-ref]

[image-ref]: image.png "Image title"
```

### Image with Link

```markdown
[![Alt text](image.png)](https://example.com)
```

## Link Best Practices

- **Meaningful link text** — `[Learn more about authentication]` not `[click here]`
- **Full URLs** for external links — always `https://`
- **Relative paths** for internal links — resolve from the containing file
- **Descriptive alt text** for images — describe content, not just "image"
- **No URL as link text** — `[Documentation](https://example.com)` not `[https://example.com]`
- **Number images sequentially** — Figure 1, Figure 2, etc.

# Alerts and Callouts

Use blockquotes with emoji and bold labels to create styled alerts and callouts.

## Note

```markdown
> 📌 **Note:** This requires administrator permissions.
```

Use for general informational notes that enhance understanding.

## Tip

```markdown
> 💡 **Tip:** Use the `--dry-run` flag to preview changes before committing.
```

Use for helpful hints, best practices, and pro tips.

## Warning

```markdown
> ⚠️ **Warning:** This operation cannot be undone.
```

Use for cautionary information that users should be aware of.

## Important

```markdown
> 🔴 **Important:** Do not delete this directory without backing up first.
```

Use for critical information that demands attention.

## Success

```markdown
> ✅ **Success:** Your changes have been deployed to production.
```

Use to confirm completed actions or positive outcomes.

## Error / Caution

```markdown
> ❌ **Error:** Invalid configuration detected.
```

Use to indicate problems or things to avoid.

## Question / Help

```markdown
> ❓ **Question:** Need help? Check the troubleshooting section.
```

Use to prompt reader engagement or offer help.

## Multi-line Callouts

```markdown
> 💡 **Tip:** For best results:
>
> - Configure authentication first
> - Test with a development environment
> - Deploy to staging before production
>
> See [Configuration Guide](config.md) for details.
```

Use blank line (`>`) between paragraphs within a blockquote.

## Styling Best Practices

- **One callout type per blockquote** — don't mix warnings and notes
- **Emoji + bold label** — follows consistency with the style guide
- **Keep concise** — blockquotes should be 1–3 lines (or a short list)
- **Place strategically** — right where the reader needs the information
- **Don't overuse** — more than one per section becomes visual noise

## GitHub Alerts (Experimental)

GitHub also supports a newer alert syntax (requires GitHub):

```markdown
> [!NOTE]
> Useful information that users should know.

> [!WARNING]
> Critical content that deserves attention.

> [!IMPORTANT]
> Essential information users must know to succeed.
```

This syntax may not render on all platforms; use the emoji+bold approach for maximum compatibility.

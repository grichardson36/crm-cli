---
type: reference
status: decided by owner 2026-10-30
---

# Web search is a choice of engines, o can offer several

**Problem.** `o` on plain text searched the web with a raw URL template in `config.toml`. Users had to know the template syntax, and one engine did not fit every kind of text (a person on LinkedIn, a place on Maps).

**Therefore.** Settings > Behavior > Web search is a choice: Google, Bing, DuckDuckGo, Brave, Ecosia, LinkedIn, Facebook, Instagram, TikTok, YouTube, X, Reddit, GitHub, Wikipedia, Google Maps, Google Images, Amazon, or Custom. Custom takes an address with `{query}` once, checked by `validate.SearchTemplate` (http or https, a host, no spaces, `E_SEARCH_TEMPLATE_INVALID`). "Web search: offer on o" marks more engines. With one engine, `o` opens it. With more than one, `o` on plain text opens the shared picker with the default first. The default is always offered. Default: Google only, no picker.

**Stored per device** in `config.toml`:

```toml
[open]
engine = "duckduckgo"
custom_url = "https://example.com/?q={query}"
offer = ["google", "github"]
```

An older `[open] search_url` is read as before and rewritten at startup: a known engine address becomes `engine`, any other becomes `custom_url` (a template without `{query}` gets it appended, as the old prefix rule did).

**Because.** Owner request 2026-10-30.

**Related.** [[features-tui-keys-and-modules]], [[input-grammar]]

**Sources.** `internal/tui/websearch.go`, `internal/tui/copyopen.go`, `internal/validate/settings.go`

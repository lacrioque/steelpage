---
title: Welcome to Steelpage
created: 2026-05-20T00:00:00+02:00
updated: 2026-05-20T00:00:00+02:00
updated_by: system
version: 1
tags:
  - welcome
---

# Welcome to Steelpage

This is your first Git-backed Markdown document.

```mermaid
flowchart TD
  A[Markdown] --> B[Go Backend Renderer]
  B --> C[Sanitized HTML]
  C --> D[Svelte SPA]
  B --> E[Print Page]
  B --> F[Bot-ready Output]
```

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, Steelpage")
}
```

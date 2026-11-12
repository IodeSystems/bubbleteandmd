# Streaming Markdown, two ways

This reply arrives **token by token**, as a model writes it. The left pane
renders it with *bubbleteandmd*: finished blocks are cached, and only the
last block renders again. Half-written syntax such as `**bold`, an open
code fence or a [link](https://github.com/iodesystems/bubbleteandmd) is
healed while it streams. The right pane renders the **whole** reply with
glamour on every token, as a program with no streaming layer does.

> [!NOTE]
> Watch the status bars: the time per frame on the right grows with the
> reply, and its height can *shrink* when a block is half written.

## Setup

1. Install the module:

   ```sh
   go get github.com/iodesystems/bubbleteandmd
   ```

2. Make one renderer per width and one `Doc` per reply:

   ```go
   r, _ := bubbleteandmd.NewRenderer(styles.DarkStyleConfig, width)
   d := bubbleteandmd.NewDoc(r, key, cache)
   out, _ := d.Frame(reply)
   ```

3. Call `Final` once, when the reply ends.

> [!TIP] Width changes
> Make a new Renderer and Doc, and copy the state: `*nd.State() = *old.State()`.

## HTTP status codes

| Code | Name                  | Class        | Retry |
|-----:|-----------------------|--------------|-------|
|  200 | OK                    | success      | no    |
|  201 | Created               | success      | no    |
|  204 | No Content            | success      | no    |
|  301 | Moved Permanently     | redirect     | no    |
|  304 | Not Modified          | redirect     | no    |
|  400 | Bad Request           | client error | no    |
|  401 | Unauthorized          | client error | no    |
|  404 | Not Found             | client error | no    |
|  409 | Conflict              | client error | maybe |
|  429 | Too Many Requests     | client error | yes   |
|  500 | Internal Server Error | server error | yes   |
|  503 | Service Unavailable   | server error | yes   |

## A long code block

```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// wordCount counts lines and words, and each word.
type wordCount struct {
	lines int
	words int
	top   map[string]int
}

func newWordCount() *wordCount {
	return &wordCount{top: map[string]int{}}
}

func (w *wordCount) add(line string) {
	w.lines++
	for _, f := range strings.Fields(line) {
		w.words++
		w.top[strings.ToLower(f)]++
	}
}

func (w *wordCount) report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "lines: %d\n", w.lines)
	fmt.Fprintf(&b, "words: %d\n", w.words)
	for word, n := range w.top {
		if n > 3 {
			fmt.Fprintf(&b, "  %-12s %d\n", word, n)
		}
	}
	return b.String()
}

func main() {
	w := newWordCount()
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		w.add(s.Text())
	}
	if err := s.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	fmt.Print(w.report())
}
```

## Wide blocks

A table wider than the pane is not wrapped; each line is cut at the pane edge.

| Region    | Instances | CPU avg | CPU p99 | Memory avg | Memory p99 | Requests/s | Errors/s | Latency p50 | Latency p99 | Owner         |
|-----------|----------:|--------:|--------:|-----------:|-----------:|-----------:|---------:|------------:|------------:|---------------|
| region-a  |        12 |     41% |     88% |     3.1 GB |     5.8 GB |      18200 |      0.4 |       12 ms |       95 ms | platform-team |
| region-b  |         8 |     37% |     79% |     2.7 GB |     5.1 GB |      11050 |      0.2 |       14 ms |      102 ms | platform-team |
| region-c  |         4 |     52% |     97% |     3.4 GB |     6.2 GB |       6400 |      1.1 |       21 ms |      188 ms | regional-ops  |

The sample numbers are made up. A long line of code is cut the same way:

```sh
curl -sS -X POST "https://api.example.com/v1/deployments?region=a&strategy=rolling&max_unavailable=1" -H "Authorization: Bearer $TOKEN" -d @deploy.json
```

## Forms

> [!TABS] Install
#### macOS
```sh
brew install thing
```
#### Linux
Use the package manager:

```sh
apt install thing
```
#### Windows
Download the installer.

> [!PROS]
- typed APIs
- fast builds

> [!CONS]
- a build step

> [!STEPS]
1. **First 24 hours**
   *Day 1*
   Rest and keep warm.
2. **Second day**
   *Day 2*
   Drink water and walk a little.

> [!STATS]
- **3×** faster installs
- **−40%** memory

```tree
src/
├── app/
│   ├── main.go  # entry
│   └── util.go
├── docs/
│   └── guide/
│       └── intro.md
└── README.md
```

> [!DETAILS] Full log
> The long part, quoted.
>
> ```text
> line one
> line two
> ```

> [!WARNING]
> Math shows as code:

$$
\sum_{i=1}^{3} x_i^2 = \frac{1}{2}
$$

> [!SOURCES]
- [llx-markdown](https://github.com/rsaryev/llx-markdown)
- [glamour](https://github.com/charmbracelet/glamour)

That is the whole reply. Press **tab** to focus the forms on the left,
**enter** to fold or switch, and **r** to stream it again.

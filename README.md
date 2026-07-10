# conflugen

CLI tool that syncs Markdown files to Confluence pages using in-file directives.

Each Markdown file declares its target Confluence page via a simple HTML comment directive — similar to how `//go:generate go-enum` annotations work. Only changed pages are updated (SHA256 hash tracking).

> **What conflugen does with the Markdown itself** (supported features, diagrams,
> multi-column layouts, includes, macros, stdlib packs) lives in
> **[`docs/FORMATTING.md`](docs/FORMATTING.md)**. A runnable example using
> every feature on one page is in
> **[`docs/examples/showcase.md`](docs/examples/showcase.md)**.

## Install

```bash
go install github.com/VereshchaginKonstantin/conflugen@latest
```

## Quick Start

1. Add a directive to your Markdown file:

```markdown
<!-- +conflugen parent-id=123456789 space-key=OB title="Архитектура сервиса" -->

# Обзор архитектуры

Сервис состоит из следующих компонентов...
```

2. Run conflugen:

```bash
conflugen -f docs/architecture.md
```

## Directive Format

Directives are HTML comments with the `+conflugen` prefix. Place them at the top of any `.md` file:

```markdown
<!-- +conflugen parent-id=123456 space-key=OB title="Custom Title" -->
```

Or split across multiple lines:

```markdown
<!-- +conflugen parent-id=123456 space-key=OB -->
<!-- +conflugen title="Custom Title" -->
```

### Parameters

| Parameter            | Required                       | Description                                                              |
|----------------------|--------------------------------|--------------------------------------------------------------------------|
| `page-id`            | one of page-id/parent-id/parent| Numeric ID of an **existing** page to update directly (PUT), skipping parent lookup and create. Use when the parent isn't listable (no view rights / index page) or creating under it is forbidden. |
| `parent-id`          | one of page-id/parent-id/parent| Numeric ID of the parent Confluence page (find-by-title under it, else create) |
| `parent`             | one of page-id/parent-id/parent| Parent page **by title**; repeatable to build an ancestry path           |
| `space-key`          | yes                     | Confluence space key                                                     |
| `title`              | no                             | Page title. With `parent-id`/`parent` defaults to filename without `.md`. With `page-id` and omitted, the page's **existing title is preserved** (not renamed). |
| `label`              | no                      | Page label; **repeatable**. Labels are synced (extras removed)           |
| `type`               | no                      | `page` (default) or `blogpost`                                           |
| `content-appearance` | no                      | `full-width` or `fixed-width`; sets both draft and published appearance  |

Specify **either** `parent-id` (exact numeric id) **or** `parent` (by title) — not both. Or use `page-id` to update an existing page directly by its id (no parent needed); `page-id` is update-only and never creates.

### Parent by title (ancestry)

Instead of looking up a numeric `parent-id`, you can address the parent page by title.
Repeat `parent` to build a hierarchy; missing ancestor pages are created automatically:

```markdown
<!-- +conflugen space-key=OB parent="Team Docs" -->
<!-- +conflugen parent="Runbooks" title="On-call" -->
```

This publishes "On-call" under `Team Docs → Runbooks`, creating either ancestor if absent.
The top-most `parent` is resolved within `space-key`; nested ones under the previous parent.

### Labels, type, content-appearance

```markdown
<!-- +conflugen parent-id=123 space-key=OB label=arch label=docs -->
<!-- +conflugen type=blogpost content-appearance=full-width -->
```

- `label` is **repeatable** and sync'd: labels missing from the directive are removed, new ones are added — running conflugen brings the page's labels to exactly the set declared in the directive.
- `type` defaults to `page`; set `type=blogpost` to publish a blog post instead.
- `content-appearance` sets both the draft and published page width via Confluence's Content Properties API. Leave it out to keep whatever is currently set in Confluence.

All `<!-- +conflugen ... -->` lines are stripped from content before publishing.

## Markdown features

Everything about **what conflugen does inside the page** — diagrams (Mermaid,
PlantUML), images (relative paths auto-uploaded as attachments), spoilers,
code blocks, `:::` multi-column layouts, includes (`+conflugen-include`),
macros (`+conflugen-macro` / `+conflugen-use`), raw `<ac:…>` passthrough —
is documented in **[`docs/FORMATTING.md`](docs/FORMATTING.md)**.

A runnable example exercising every feature on one page is in
**[`docs/examples/showcase.md`](docs/examples/showcase.md)** — use it as a
test document for your Confluence integration.

## Usage

> **Important:** You must specify your Confluence URL via `--url` flag or `CONFLUENCE_URL` environment variable.
> The default value (`confluence.example.com`) is a placeholder and will not work.

```bash
# Set Confluence URL (do this once per shell session)
export CONFLUENCE_URL="https://confluence.your-company.com/rest/api"
export CONFLUENCE_TOKEN="your-token-here"

# Process specific files
conflugen -f docs/architecture.md -f docs/api.md

# Positional arguments also work
conflugen docs/architecture.md docs/api.md

# Dry run — see what would happen without making changes
conflugen -f docs/architecture.md --dry-run

# Explicit URL via flag
conflugen -f docs/architecture.md --url https://confluence.your-company.com/rest/api

# Debug mode (verbose Confluence API output)
conflugen -f docs/architecture.md --debug
```

### Flags

| Flag            | Description                                            | Default                              |
|-----------------|--------------------------------------------------------|--------------------------------------|
| `-f`            | Markdown file to process (repeatable)                  | —                                    |
| `--token`       | Confluence Personal Access Token (or `CONFLUENCE_TOKEN`); used as Bearer | —          |
| `--user`        | Confluence username for basic auth (or `CONFLUENCE_USER`); empty → Bearer | —             |
| `--password`    | Confluence password (or `CONFLUENCE_PASSWORD`); pairs with `--user` for Basic | —         |
| `--url`         | Confluence REST API URL (or `CONFLUENCE_URL`)          | `https://confluence.example.com/rest/api` |
| `--dry-run`     | Preview mode, no changes                               | `false`                              |
| `--debug`       | Verbose Confluence API output                          | `false`                              |
| `--request-interval` | Minimum delay between Confluence requests, guards against HTTP 429 (or `CONFLUENCE_REQUEST_INTERVAL`); e.g. `300ms`, `1s`; `0` disables | `300ms` |

## Downloading pages

`conflugen download` выгружает страницы Confluence в локальную папку: содержимое
в storage format как есть, плюс все вложения.

```bash
# pages.txt: по одному pageId на строку, # — комментарий
cat > pages.txt <<'EOF'
# корневые страницы для выгрузки
123456789
123456790
EOF

conflugen download --list pages.txt --out ./dump
```

Обход идёт **вглубь**: по дочерним страницам и по ссылкам на другие страницы,
найденным в тексте. Ограничений по глубине и числу страниц нет — остановка по
`Ctrl+C`. Повторный запуск в ту же папку продолжает с того же места: страницы,
уже выгруженные в актуальной версии, пропускаются.

Раскладка:

```
dump/
├─ 123456789-arhitektura-servisa/
│  ├─ page.xhtml        # storage format, байт в байт
│  ├─ page.json         # id, title, space, version, метки, предки, ссылка
│  └─ attachments/
│     └─ diagram.svg
└─ index.json           # карта дампа: страницы, ошибки, рёбра краула
```

Файлы пишутся атомарно (`tmp` + `rename`), а `page.json` — последним, после тела
и всех вложений. Поэтому `Ctrl+C` в любой момент оставляет либо целую страницу,
либо ничего: обрезанного XHTML или битого PNG в дампе не бывает, и следующий
запуск докачает то, что не успело записаться.

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--list` | Файл со списком pageId (repeatable) | — (required) |
| `--out` | Папка для выгрузки | — (required) |
| `--force` | Перекачивать уже выгруженные страницы | `false` |
| `--request-interval` | Минимальный интервал между запросами | `2s` |

Аутентификация, `--url`, `--user-agent` и `--debug` — те же флаги и переменные
окружения, что у публикации.

Интервал по умолчанию (`2s`) заметно больше публикаторских `300ms`: у выгрузки
запросов на порядок больше — страница, её метки, список вложений и по запросу на
каждый файл, — и плотная серия упирается в HTTP 429.

### Troubleshooting: `dial tcp: lookup confluence.example.com: no such host`

This error means you did not set the `--url` flag. The default URL is a placeholder.

**Fix:** specify your Confluence instance URL:

```bash
# Via environment variable (recommended — set once in .env or .bashrc)
export CONFLUENCE_URL="https://confluence.your-company.com/rest/api"
conflugen -f docs/file.md

# Or via flag
conflugen -f docs/file.md --url https://confluence.your-company.com/rest/api
```

### Confluence URL

conflugen needs the REST API URL of your Confluence instance.

**How to find your Confluence URL:**

1. Open any page in your Confluence in a browser
2. Look at the address bar — the base URL is everything before `/display/`, `/pages/`, or `/spaces/`
3. Append `/rest/api` to get the API URL

Examples:

| Browser address bar                                          | `CONFLUENCE_URL` value                          |
|--------------------------------------------------------------|-------------------------------------------------|
| `https://confluence.your-company.com/display/TEAM/Page`      | `https://confluence.your-company.com/rest/api`  |
| `https://wiki.example.org/pages/viewpage.action?pageId=123`  | `https://wiki.example.org/rest/api`             |
| `https://mycompany.atlassian.net/wiki/spaces/DEV`            | `https://mycompany.atlassian.net/wiki/rest/api` |

Set it once via environment variable:

```bash
export CONFLUENCE_URL="https://confluence.your-company.com/rest/api"
```

Or in `.env` file:

```bash
CONFLUENCE_URL=https://confluence.your-company.com/rest/api
```

### Authentication

conflugen supports two authentication modes — pick the one your Confluence instance accepts:

| Mode                       | Flags                                | Header                                   |
|----------------------------|--------------------------------------|------------------------------------------|
| Personal Access Token (PAT)| `--token PAT`                        | `Authorization: Bearer <token>`          |
| Basic auth (login+password)| `--user LOGIN --password PASSWORD`   | `Authorization: Basic base64(user:pass)` |

Switching is automatic: if `--user` (or `CONFLUENCE_USER`) is **empty** — conflugen uses **Bearer**; otherwise — **Basic**. For backward compatibility, `--token` (or `CONFLUENCE_TOKEN`) is accepted as a fallback for the password when `--password` is not provided.

---

#### Option A — Personal Access Token (Bearer)

[Personal Access Token docs](https://confluence.atlassian.com/enterprise/using-personal-access-tokens-1026032365.html)

**How to get a token:**

1. Open Confluence → click your avatar (top right) → **Settings**
2. In the left menu select **Personal Access Tokens**
3. Click **Create token**, name it `conflugen`, set expiration, click **Create**
4. Copy the token (it is shown only once)

**How to provide it:**

```bash
# Recommended — env var
export CONFLUENCE_TOKEN="your-pat-here"
conflugen -f docs/architecture.md

# Or via flag (not recommended for shared shells)
conflugen -f docs/architecture.md --token "your-pat-here"
```

---

#### Option B — Login + password (Basic auth)

Use this if your Confluence rejects Bearer (returns 401/403 even with a fresh PAT) or you must authenticate as a service account.

```bash
# Recommended — env vars
export CONFLUENCE_USER="your-login"
export CONFLUENCE_PASSWORD="your-password-or-api-token"
conflugen -f docs/architecture.md

# Or via flags
conflugen -f docs/architecture.md --user your-login --password your-password
```

`--password` here is either your actual password or, for Confluence Cloud, an API token from <https://id.atlassian.com/manage-profile/security/api-tokens>.

---

`.env` file (add to `.gitignore`!):

```bash
# .env
CONFLUENCE_URL=https://confluence.your-company.com/rest/api

# Bearer (PAT)
CONFLUENCE_TOKEN=your-pat-here

# Or Basic auth (uncomment instead of the line above)
# CONFLUENCE_USER=your-login
# CONFLUENCE_PASSWORD=your-password
```

```bash
source .env && conflugen -f docs/architecture.md
```

In `--dry-run` mode no credentials are required.

## Makefile Integration

### Install target (like go-enum)

Add an install target to your Makefile, similar to how `go-enum` is installed:

```makefile
CONFLUGEN_VERSION ?= latest

.PHONY: install-conflugen
install-conflugen:
	go install github.com/VereshchaginKonstantin/conflugen@$(CONFLUGEN_VERSION)
```

### Generate docs target

```makefile
.PHONY: generate-docs
generate-docs: install-conflugen
	conflugen \
		-f docs/architecture.md \
		-f docs/api-reference.md \
		-f docs/runbook.md
```

### Full example (go-enum + conflugen)

```makefile
GO_ENUM_VERSION ?= v0.6.0
CONFLUGEN_VERSION ?= latest

# --- Install tools ---

.PHONY: install-goenum
install-goenum:
	go install github.com/abice/go-enum@$(GO_ENUM_VERSION)

.PHONY: install-conflugen
install-conflugen:
	go install github.com/VereshchaginKonstantin/conflugen@$(CONFLUGEN_VERSION)

.PHONY: bin-deps
bin-deps: install-goenum install-conflugen

# --- Generate ---

.PHONY: generate-enums
generate-enums: install-goenum
	go-enum -f internal/pkg/worker/types.go --nocase --marshal --sql

.PHONY: generate-docs
generate-docs: install-conflugen
	conflugen \
		-f docs/architecture.md \
		-f docs/api-reference.md \
		-f docs/runbook.md

.PHONY: generate-docs-dry
generate-docs-dry: install-conflugen
	conflugen --dry-run \
		-f docs/architecture.md \
		-f docs/api-reference.md

.PHONY: generate
generate: generate-enums generate-docs
```

### Usage

```bash
# Install conflugen
make install-conflugen

# Publish docs to Confluence
make generate-docs

# Dry run (preview without changes)
make generate-docs-dry

# All generation (enums + docs)
make generate
```

## GitLab CI/CD Integration

Publish docs automatically from a GitLab pipeline. Add `CONFLUENCE_URL` and `CONFLUENCE_TOKEN` as CI/CD variables (Settings → CI/CD → Variables; mark `CONFLUENCE_TOKEN` as **masked** and **protected**).

`Makefile`:

```makefile
LOCAL_BIN := $(CURDIR)/bin

.PHONY: .bin-conflugen
.bin-conflugen:
	GOBIN=$(LOCAL_BIN) go install github.com/VereshchaginKonstantin/conflugen@latest

.PHONY: publish-docs
publish-docs: .bin-conflugen
	$(LOCAL_BIN)/conflugen -f docs/architecture.md
```

`.gitlab-ci.yml`:

```yaml
publish-docs:
  stage: build
  allow_failure: true
  script:
    - make publish-docs
  rules:
    - if: '$CI_COMMIT_BRANCH && $CI_COMMIT_REF_NAME !~ "master"'
      when: on_success
    - when: never
```

Notes:

- `allow_failure: true` keeps a broken publish from blocking the rest of the pipeline while the integration stabilises.
- The rule above runs on every branch **except** `master` — useful for testing the job from feature branches before flipping it on the default branch. To publish only after merge to the default branch, invert the condition (`$CI_COMMIT_BRANCH == "master"`) and trigger via a merge-to-master pipeline.
- No GitLab runner setup is conflugen-specific — any Go-capable runner image works (the `Makefile` target installs the binary on the fly via `go install`).

## How It Works

1. Reads each specified `.md` file
2. Expands `<!-- +conflugen-include path -->` directives recursively
3. Collects macros (`<!-- +conflugen-macro ... -->`, `<!-- +conflugen-use ... -->`)
4. Parses `<!-- +conflugen ... -->` directives
5. Applies macros to the cleaned body
6. Converts Markdown to Confluence Storage Format (HTML/XML)
7. Computes SHA256 hash of the HTML content
8. Creates or updates the Confluence page (skips if hash matches)
9. Syncs labels and content-appearance (when set)
10. Appends a footer with "auto-generated" note and hash for change detection

### Inline Comment Preservation

When updating an existing page, conflugen preserves inline comments (text highlight + comment) that were added in the Confluence UI. Since the page content is fully replaced, inline comments lose their text anchors. conflugen handles this by:

1. Reading all inline comments from the page before the update
2. Updating the page content
3. Re-creating saved comments as regular page comments with the original quoted text

Each restored comment includes the author name, the original highlighted text (as a blockquote), and the comment body:

> **[Комментарий от Author Name, перенесён conflugen]:**
> > highlighted text fragment
>
> comment body

## Change Detection

Each published page includes a hidden hash macro:

```
conflugen-hash:<sha256>
```

On subsequent runs, conflugen compares the hash — if content hasn't changed, the page is skipped. This prevents unnecessary version bumps in Confluence.

# Contributing to this project

## Workflow

All changes travel through GitHub PRs:

1. **Create an issue first** — before any branch or code
2. **Branch** from `main`
3. **Commit** with a conventional message referencing the issue: `fix(#N): description`, `feat(#N): description`
4. **Open a PR** with `Closes #N` in the body
5. **Self-review** the diff before requesting review
6. **Merge** via squash merge (fast-forward merge for hotfixes)
7. **Delete** the branch after merge

## Code style

- Go: run `go fmt` and `go vet` before committing
- Python: run `black` and `ruff check`
- Shell: shellcheck for shell scripts

## Tests

- New features must include tests
- Bug fixes must include a regression test
- Run the full test suite before opening a PR:
  ```bash
  go test ./...   # Go projects
  python3 -m pytest  # Python projects
  ```

## Commit message format

```
<type>(<scope>): <short description>

[optional body]

Closes #<issue-number>
```

Types: `fix`, `feat`, `docs`, `chore`, `refactor`, `test`

## Branch protection

- `main` is protected; force-push is disabled
- All PRs require at least one review before merge
- CI must pass before merge

## OpenSpec

This project uses OpenSpec for change planning. All non-trivial changes go through the OpenSpec workflow before implementation:

1. Explore the problem: `/opsx-explore`
2. Propose the change: `/opsx-propose`
3. Implement from the change: `/opsx-apply`
4. Archive after merge: `/opsx-archive`

Run `openspec list` to see active changes.

## Questions

Open an issue for discussion before starting work on non-trivial changes.

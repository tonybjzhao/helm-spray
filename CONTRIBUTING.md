# Contributing guidelines

Thanks for your interest in improving helm-spray!

## Filing issues

Use the GitHub issue tracker with the bug-report or feature-request template.
For bugs, include your helm, kubectl and helm-spray versions.

## Submitting a change

1. Open an issue describing the change so it can be discussed first.
2. Fork the repository and create a topic branch.
3. Make your change with tests, and ensure the following all pass:
   - `go build ./...`
   - `go vet ./...`
   - `go test ./...`
   - `gofmt -l .` reports no files
4. Sign off your commits to certify the
   [Developer Certificate of Origin](https://developercertificate.org):
   commit with `git commit -s`.
5. Open a pull request and complete the checklist in the template.

## Conventions

- Keep commits focused and write clear, imperative commit messages.
- Do not reference competing deployment tools by name in code, documentation, or
  commit messages.

# Agent Instructions

Before marking a coding task done, creating a commit, or saying work is ready, run the project's basic validations:

- `make test`
- `make build`
- `make lint-once`

If code behavior changed, add or update focused tests for the changed or new lines before running validation.

When coverage is relevant, also run:

- `make coverage`

Do not claim validation passed unless the commands actually ran successfully. If a command was not run, explain why.

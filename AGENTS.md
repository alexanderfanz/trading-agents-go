# Agent Instructions

Before marking a coding task done, creating a commit, or saying work is ready, run the project's basic validations:

- `make test`
- `make build`
- `make lint-once`

Treat validation failures as actionable. Investigate failing tests, build errors, and lint findings, then fix every issue you can safely address within the task. After making fixes, rerun the failing command and any later validation commands.

If code behavior changed, add or update focused tests for the changed or new lines before running validation.

When coverage is relevant, also run:

- `make coverage`

Do not claim validation passed unless the commands actually ran successfully. If a command was not run, or a failure could not be fixed, explain why and describe the remaining issue.

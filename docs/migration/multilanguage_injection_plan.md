# Multi-Language Injection Plan

This document outlines the design and implementation details for migrating the multi-language system prompt localization mechanism from the sibling Python repository `TradingAgents` to `trading-agents-go`.

---

## 1. Analysis of Python Reference Implementation

In the Python codebase (`tradingagents/agents/utils/agent_utils.py`), the function `get_language_instruction` is defined as follows:

```python
def get_language_instruction() -> str:
    from tradingagents.dataflows.config import get_config
    lang = get_config().get("output_language", "English")
    if lang.strip().lower() == "english":
        return ""
    return f" Write your entire response in {lang}."
```

### Key Behaviors:
- **Default Language**: Defaults to `"English"` if the configuration value is missing or matches `"English"` (case-insensitive, trimmed).
- **Return Value**: 
  - If the language is English, returns an empty string `""` to save context tokens.
  - If a non-English language is specified (e.g., `"Spanish"`, `"Chinese"`), returns `" Write your entire response in <Language>."` with a leading space.
- **Propagation**: This localized instruction is appended to the system prompts of all active agent nodes (analysts, researchers, debaters, research manager, trader, and portfolio manager) to force the LLM to output entirely in the selected language.

---

## 2. Evaluation of Go Codebase

In the Go implementation:
- `internal/config/config.go` successfully parses `TRADINGAGENTS_OUTPUT_LANGUAGE` (defaulting to `"English"`) and maps it to `config.Config.OutputLanguage`.
- Currently, this `OutputLanguage` field is **completely ignored**, and all system prompts are hardcoded in English inside `internal/orchestrator/orchestrator.go`.

### Active Agent Nodes Identified in `internal/orchestrator/orchestrator.go`:
1. **Market Analyst**: `NewAgent("Market Analyst", ...)`
2. **Sentiment Analyst**: `NewAgent("Sentiment Analyst", ...)`
3. **News Analyst**: `NewAgent("News Analyst", ...)`
4. **Fundamentals Analyst**: `NewAgent("Fundamentals Analyst", ...)`
5. **Bull Analyst**: `NewAgent("Bull Analyst", ...)`
6. **Bear Analyst**: `NewAgent("Bear Analyst", ...)`
7. **Research Manager** (JSON-producing): `NewAgent("Research Manager", ...)`
8. **Trader** (JSON-producing): `NewAgent("Trader", ...)`
9. **Aggressive Risk Analyst**: `NewAgent("Aggressive Risk", ...)`
10. **Conservative Risk Analyst**: `NewAgent("Conservative Risk", ...)`
11. **Neutral Risk Analyst**: `NewAgent("Neutral Risk", ...)`
12. **Portfolio Manager** (JSON-producing): `NewAgent("Portfolio Manager", ...)`

---

## 3. Proposed Go Implementation

### A. Localization Helper Utility
We will create a new utility file at `internal/orchestrator/localization.go` to house the language instruction formatter:

```go
package orchestrator

import "strings"

// getLanguageInstruction maps the output language configuration to a prompt suffix.
// It returns an empty string when the language is English or empty, avoiding unnecessary token usage.
func getLanguageInstruction(lang string) string {
	trimmed := strings.TrimSpace(lang)
	if trimmed == "" || strings.ToLower(trimmed) == "english" {
		return ""
	}
	return " Write your entire response in " + trimmed + "."
}
```

### B. Safe Prompt Interpolation Design
To inject this instruction safely into the agent prompts while preserving structured JSON schema requirements, we will introduce a private helper method on the `TradingOrchestrator` struct. 

Since every single agent instantiation in `orchestrator.go` passes `o.llmProvider` as the final argument, we can centralize this creation:

```go
// createAgent instantiates a new Agent with the configured output language suffix automatically appended to its system prompt.
func (o *TradingOrchestrator) createAgent(name, role, baseInstruction string) *Agent {
	langSuffix := getLanguageInstruction(o.cfg.OutputLanguage)
	return NewAgent(name, role, baseInstruction+langSuffix, o.llmProvider)
}
```

#### Why This is Safe and Robust:
1. **Preserves Schema Rules**: For structured agents (Research Manager, Trader, Portfolio Manager), appending `" Write your entire response in <Language>."` to the system prompt forces the LLM to output the key-value text in the target language while maintaining the structural layout of the requested JSON schema.
2. **Cohesive and Simple**: Standardizes all 12 agent instantiations with a single pattern, avoiding tedious string operations in the execution phases.
3. **No Signature Changes**: `NewAgent` remains unmodified to prevent side effects in other files/tests, and its type safety is intact.

---

## 4. Verification & Testing Strategy

### Unit Testing
We will add a new test suite under `internal/orchestrator/localization_test.go` that:
1. Validates the behavior of `getLanguageInstruction` against several inputs:
   - `"English"` (and varying cases like `"english"`, `"  ENGLISH  "`) should return `""`.
   - Empty string `""` should return `""`.
   - Other languages like `"Spanish"`, `"Chinese"`, `"German"` should return `" Write your entire response in <Language>."`.
2. Verifies that the config successfully propagates and modifies system instructions of agents produced via the orchestrator.

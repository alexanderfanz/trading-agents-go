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

// createAgent instantiates a new Agent with the configured output language suffix automatically appended to its system prompt.
func (o *TradingOrchestrator) createAgent(name, role, baseInstruction string) *Agent {
	langSuffix := getLanguageInstruction(o.cfg.OutputLanguage)
	return NewAgent(name, role, baseInstruction+langSuffix, o.llmProvider)
}

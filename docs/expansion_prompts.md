# TradingAgents: Rewrite Specification Expansion Prompts

This document provides a set of **six highly granular, instruction-dense prompts** (one for the Master Coordinator and one for each major Component design file). These prompts are designed to instruct an AI coding assistant (or guide a senior systems architect) to expand, refine, and inject advanced, real-world implementation details into each respective plan.

---

## Prompt 1: Master Coordinator Plan Expansion
**Target File**: `docs/general_implementation_plan.md`

```markdown
Role: Principal Go Software Architect
Task: Expand and enrich 'docs/general_implementation_plan.md' with hyper-advanced runtime orchestration and error-recovery patterns.

Please modify the master coordinator plan to inject the following technical specifications:
1. **Goroutine Panics & Recovery Protocol**: Define a precise recovery middleware/defer block inside the Orchestrator. Specify how to trap panics inside concurrent analyst goroutines, recover gracefully, map the runtime stack trace to a structured error container, and prevent a single analyst panic from crashing the entire trading program.
2. **Context Deadline Cascading**: Explain how a master context timeout (e.g., 60 seconds) propagates down through individual components. Show a detailed diagram illustrating how context deadlines are divided: e.g., 20 seconds for parallel data fetching, 15 seconds for each LLM provider call, and 5 seconds for SQLite transaction writes.
3. **Advanced State Versioning & Compatibility Matrix**: Add a schema versioning design for the serialized state data (Gob/JSON). Define how the Orchestrator handles migrating legacy saved checkpoint states if a new release changes structural field definitions (e.g. adding a new analyst report type) during an active resumed run.
4. **End-to-End System Benchmark Strategy**: Provide a complete test specification detailing how to run high-load parallel stress tests on the Orchestrator using Go's `testing.B` benchmark routines. Detail how to simulate mock network failures and verify memory allocations during debate loops.
```

---

## Prompt 2: Component 1 (Data & Indicators) Expansion
**Target File**: `docs/component1_dataflows.md`

```markdown
Role: Senior High-Performance Go Engineer
Task: Expand 'docs/component1_dataflows.md' with low-level memory optimizations, strict CSV parsing pipelines, and advanced rate-limiting designs.

Please update the Component 1 design file to include the following detailed implementations:
1. **Dynamic Indicators Math Derivations**: Add the exact mathematical derivations and Go loop step boundaries for MFI (Money Flow Index), ATR (Average True Range), and Bollinger Bands. Provide the in-place slice mutation signatures and error bounds checking.
2. **SIMD-Friendly Optimization Strategies**: Detail how to layout float slices to maximize CPU L1/L2 cache locality during tight mathematical loops. Provide concrete compiler guidelines (e.g., avoiding bounds checks via slice pattern slicing `_ = prices[len-1]`) to encourage the Go compiler to vectorize the loops.
3. **Robust CSV Stream Tokenization**: Write a complete Go implementation of the Yahoo Finance HTTP reader utilizing a buffered `bufio.Reader` and a custom row tokenization scanner to process CSV columns dynamically without loading the entire raw file into heap memory.
4. **Token Bucket Rate Limiter with Jitter**: Provide the exact struct layout and synchronization code for a thread-safe `TokenBucket` rate-limiter in Go. Show how to configure exponential backoff retries with full jitter algorithms to safely handle API rate-limit drops on multi-symbol sweeps.
```

---

## Prompt 3: Component 2 (ADK Orchestration) Expansion
**Target File**: `docs/component2_orchestration.md`

```markdown
Role: Senior AI Systems Architect (Go + ADK)
Task: Expand 'docs/component2_orchestration.md' with advanced channel multiplexing, prompt routing protocols, and ADK integration models.

Please refine the Component 2 design file to inject the following specifications:
1. **Goroutine Fan-In / Fan-Out Channel Architecture**: Provide the complete, production-grade Go code block showing how to fan-out analysts using a bidirectional channel pipeline. Implement a multiplexer ('fan-in') that collects analyst outputs into a thread-safe map, tracking execution latency per routine.
2. **Google ADK Lifecycle Hooks**: Provide a detailed schema explaining the lifecycle of `adk.Agent` execution in Go. Show how to configure system instructions, register dynamic indicator tools to the agent context, and serialize long-running multi-turn debate loops natively.
3. **Dynamic Consensus Scoring Engine**: Design a procedural consensus evaluation routine for the Bull/Bear debate. Instead of a simple max-rounds cutoff, specify how a consensus scorer uses regex or JSON parsing to inspect the Research Manager's recommendation confidence, triggering early loop termination if convergence is achieved.
4. **Full Mock Execution Pipeline**: Write a complete mock implementation of the parallel analyst and debate routines inside a Go unit test script. Demonstrate how to execute the orchestrator against a mock ADK client that simulates high-latency model responses.
```

---

## Prompt 4: Component 3 (Unified Provider Facade) Expansion
**Target File**: `docs/component3_providers.md`

```markdown
Role: Senior API Integration Engineer
Task: Expand 'docs/component3_providers.md' with concrete client configurations, schema converters, and strict type validation patterns.

Please update the Component 3 design document to incorporate the following implementation specifics:
1. **Anthropic Adapter Tool Schema Integration**: Write the exact Go concrete adapter structure for `github.com/charmbracelet/lipgloss` or `anthropics-sdk-go` that translates complex struct schemas into Anthropic's tool-use parameter arrays.
2. **Gemini GenAI V1 Schema Conversions**: Provide a complete, explicit helper function that converts standard Go struct reflect typings dynamically into the official `*genai.Schema` structure format. Ensure it handles nested arrays and optional object pointers.
3. **Structured Output Fallback Parsing**: Detail the recovery steps when a provider fails to yield valid structured JSON. Show how to write a regex-based fallback extractor that pulls JSON blocks out of raw conversational strings if the model slips into non-structured mode.
4. **Robust Middleware Interceptors**: Implement custom HTTP round-tripper logging middlewares for OpenAI and Gemini adapters. Specify how to capture model token usage (prompt and completion tokens), network request durations, and serialize request/response payloads to a debug folder.
```

---

## Prompt 5: Component 4 (Database & Checkpointer) Expansion
**Target File**: `docs/component4_database.md`

```markdown
Role: Go Database & Systems Engineer
Task: Expand 'docs/component4_database.md' with WAL configurations, database locks, and highly-optimized serialization plans.

Please refine the Component 4 design document to inject the following technical systems specs:
1. **Advanced SQLite WAL Mode Tuning**: Provide the exact SQL connection pool parameters and dynamic configuration values. Explain how to manage database transaction locks (`BEGIN IMMEDIATE`) programmatically to prevent "database is locked" errors during parallel state updates.
2. **Dynamic Gob vs. Gzip JSON Benchmark Analysis**: Add a comprehensive comparative analysis of binary GOB serialization versus gzip-compressed JSON. Provide structural layouts and benchmarking metrics showing file read/write sizes and de-serialization latencies.
3. **State Rollback & Resumption Validation Rules**: Detail the state validation steps run immediately after a checkpoint load. Define how to verify struct parameters (e.g., verifying that the loaded `trade_date` matches the run arguments) to reject corrupted or stale checkpoint rows.
4. **Automatic Database Vacuum & Pruning Schedule**: Specify a programmatic cleanup routine inside the SQL manager. Design a vacuum operation that automatically executes if the sqlite DB size exceeds a pre-set ceiling, alongside standard cron deletion paths for completed checkpoints.
```

---

## Prompt 6: Component 5 (Lipgloss Linear CLI) Expansion
**Target File**: `docs/component5_cli.md`

```markdown
Role: Premium UI/UX Engineer (Charmbracelet & Lipgloss)
Task: Expand 'docs/component5_cli.md' with HSL typography rules, grid card layouts, and non-blocking spinner mechanics.

Please update the Component 5 design file to include the following UI implementation specifications:
1. **Obsidian Theme Color Tokens**: Define the exact hex color codes and HSL channels representing the obsidian glass theme. Show how to construct dynamic border colors that shift from muted slate to glowing mint depending on state indicators.
2. **Complex Table Grid Formatting**: Write a complete Go code block utilizing Charmbracelet's `lipgloss.JoinHorizontal` and `lipgloss.JoinVertical` methods to display comparative technical indicators side-by-side inside HSL-styled grids.
3. **ANSI Carriage Return Spinner Mechanics**: Detail the system-level console mechanics of the progress spinner. Explain how to use `\r` carriage returns and ANSI `\033[K` clear-line operations to animate spinners smoothly on standard terminals without corrupting the output scroll buffer.
4. **Piping & Redirection Automatic Detection**: Provide the exact implementation of a check using `golang.org/x/term` (e.g. `term.IsTerminal(int(os.Stdout.Fd()))`). Show how the Linear CLI automatically disables ANSI escape codes, colored cards, and loading spinners if it detects that stdout is piped directly to a file.
```

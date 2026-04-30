# Embedded LLM Spike — Deferred

## Context

The CLI local-first pivot plan (Phase 3 / BYOK LLM) included an exploratory
spike to evaluate running small GGUF/ONNX models directly in `kleio-cli`,
enabling LLM-powered features (summarization, semantic search) without any
external API keys.

## Candidates Evaluated

| Runtime | Format | Go Integration |
|---------|--------|----------------|
| llama.cpp via CGo | GGUF | [go-llama.cpp](https://github.com/go-skynet/go-llama.cpp) — requires C compiler, CGo, platform-specific builds |
| ONNX Runtime | ONNX | [onnxruntime-go](https://github.com/yalue/onnxruntime_go) — C shared library dependency |
| llamafile | GGUF | Ships as standalone binary; usable via subprocess + HTTP API |

## Decision

**Deferred.** Embedding LLMs into the CLI binary was deprioritized for the
following reasons:

1. **Binary size**: Even quantized models (Q4_K_M) add 2–4 GB, making the CLI
   impractically large for a zero-friction install.
2. **CGo / build complexity**: Both go-llama.cpp and onnxruntime-go require CGo
   and platform-specific C toolchains, breaking the single-binary cross-compile
   story that Go provides.
3. **Runtime RAM**: Inference on a 7B-parameter model requires ~4 GB RAM,
   exceeding reasonable defaults for a CLI tool.
4. **BYOK covers the need**: Users who want LLM features can configure an API
   key (OpenAI, Anthropic, Gemini) or point to a local Ollama instance, which
   handles model management independently.
5. **Time budget**: The spike exceeded the allocated 3-iteration timebox without
   producing a viable zero-dependency integration path.

## Recommendation

If embedded LLM support is revisited:

- **llamafile** is the most promising path: ship it as an optional sidecar
  binary and communicate over its local HTTP API. This avoids CGo entirely.
- Monitor [Ollama's embedding API](https://ollama.com) — as Ollama adoption
  grows, assuming users have it installed becomes more viable than embedding.
- Tiny models (< 1B params) may become practical as quantization improves.
  Re-evaluate when sub-500 MB GGUF models with acceptable summarization quality
  emerge.

package prompts

// Canonical template names. These DO MATCH the filenames under
// internal/ai/prompts/templates/<name>.prompt
// The CORE key from Registry.Get
const (
	NameRewrite        = "rewrite"
	NameClassifyStage1 = "classify_stage1"
	NameClassifyStage2 = "classify_stage2"
	NameHyDE           = "hyde"
	NameFusion         = "fusion"
	NameRerank         = "rerank"
	NameCRAG           = "crag"
	NameSelfRAG        = "self_rag"
	NameGenerate       = "generate"
	NameChat           = "chat"
	NameChatStream     = "chat_stream"
	NameToolSummary    = "tool_summary"
	NameClarify        = "clarify"
	NameIntakeExtract  = "intake_extract"
)

// MUST be presented along with template coveraging
// Registry construction
func canonicalNames() []string {
	return []string{
		NameRewrite,
		NameClassifyStage1,
		NameClassifyStage2,
		NameHyDE,
		NameFusion,
		NameRerank,
		NameCRAG,
		NameSelfRAG,
		NameGenerate,
		NameChat,
		NameChatStream,
		NameToolSummary,
		NameClarify,
		NameIntakeExtract,
	}
}

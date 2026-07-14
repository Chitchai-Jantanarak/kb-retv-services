package prompts

// Canonical template names. These match the filenames under
// internal/ai/prompts/templates/<name>.prompt and are also the keys
// used by Registry.Get.
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
	NameIntakeExtract  = "intake_extract"
)

// canonicalNames lists every template that must be present in any
// loaded set. Tests assert coverage; Registry construction fails when
// a canonical template is missing from the loaded source.
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
		NameIntakeExtract,
	}
}

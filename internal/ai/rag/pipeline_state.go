package rag

type pipelineState struct {
	originalQuery    Query
	query            Query
	plan             QueryPlan
	cacheKey         string
	timings          map[string]int64
	candidates       []Candidate
	contextAssembled bool
	cragResult       CRAGResult
	verdict          CRAGVerdict
	confidence       float64
	threshold        float64
	decision         Decision
	draft            string
	generationErr    error
	critique         CritiqueResult
	criticRan        bool
	reason           Reason
	retryAfterMS     int64
}

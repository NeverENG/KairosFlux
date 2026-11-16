package researchdata

// StrategyPhase 是策略生命周期的存储标记；阶段推进和实盘准入由上游负责。
type StrategyPhase string

const (
	StrategyHypothesis StrategyPhase = "hypothesis"
	StrategyGate       StrategyPhase = "gate"
	StrategyCandidate  StrategyPhase = "candidate"
	StrategyPaper      StrategyPhase = "paper"
	StrategyLive       StrategyPhase = "live"
	StrategyRetired    StrategyPhase = "retired"
)

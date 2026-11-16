package aiplane

import (
	"encoding/json"
	"fmt"
	"math"
)

// SimilarityEdge 是两个因子之间的收益相关性边（方案 §3.2 对象模型
// FactorSimilarity kind）。FactorA/FactorB 落盘时按字符串字典序排列，
// 使同一对因子无论调用方以什么顺序传参，都落在同一个逻辑键、编码结果
// 逐字节相同（无向边）。
type SimilarityEdge struct {
	FactorA          string  `json:"factor_a"`
	FactorB          string  `json:"factor_b"`
	Correlation      float64 `json:"correlation"`
	SuspectDuplicate bool    `json:"suspect_duplicate"`
}

// SimilarityKey 返回 factor:similarity:{a}|{b} 逻辑键，a/b 按字典序排列。
// 用 "|" 而不是 ":" 分隔两个因子名，避免因子名本身含 ":" 时产生切分歧义
// （因子名目前的实际取值如 "amihud_illiq" 不含 "|"，但键布局本身的分隔符
// 选择不应该依赖"目前恰好没有"这种偶然事实）。
func SimilarityKey(factorA, factorB string) string {
	a, b := factorA, factorB
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("factor:similarity:%s|%s", a, b)
}

// StoreSimilarity persists a correlation and upstream-provided review flag without
// computing factor returns or deciding which factor may enter the evidence graph.
// The key and JSON payload remain compatible with previously stored edges.
func StoreSimilarity(rw ReadWriter, edge SimilarityEdge) (SimilarityEdge, error) {
	if edge.FactorA == "" || edge.FactorB == "" || edge.FactorA == edge.FactorB {
		return SimilarityEdge{}, fmt.Errorf("similarity requires two distinct nonempty factors")
	}
	if math.IsNaN(edge.Correlation) || math.IsInf(edge.Correlation, 0) || edge.Correlation < -1 || edge.Correlation > 1 {
		return SimilarityEdge{}, fmt.Errorf("correlation must be finite and between -1 and 1")
	}
	if edge.FactorA > edge.FactorB {
		edge.FactorA, edge.FactorB = edge.FactorB, edge.FactorA
	}
	payload := encodeSimilarityEdge(edge)
	key := SimilarityKey(edge.FactorA, edge.FactorB)
	existing, found, err := rw.GetAsOf(key, nowUnboundedAsOf)
	if err != nil {
		return SimilarityEdge{}, fmt.Errorf("读现有相似度边失败: %w", err)
	}
	if found && string(existing) == string(payload) {
		return edge, nil
	}
	if _, err := WriteAsEngine(rw, KindFactorSimilarity, key, payload); err != nil {
		return SimilarityEdge{}, fmt.Errorf("写入相似度边失败: %w", err)
	}
	return edge, nil
}

// LookupSimilarity 读取 factorA/factorB 之间已记录的相似度边；未记录返回
// found=false。
func LookupSimilarity(reader AsOfReader, factorA, factorB string, asOfNanos int64) (edge SimilarityEdge, found bool, err error) {
	raw, ok, err := reader.GetAsOf(SimilarityKey(factorA, factorB), asOfNanos)
	if err != nil {
		return SimilarityEdge{}, false, err
	}
	if !ok {
		return SimilarityEdge{}, false, nil
	}
	e, decodeOK := decodeSimilarityEdge(raw)
	if !decodeOK {
		return SimilarityEdge{}, false, fmt.Errorf("相似度边解码失败: %s", SimilarityKey(factorA, factorB))
	}
	return e, true, nil
}

func encodeSimilarityEdge(e SimilarityEdge) []byte {
	out, _ := json.Marshal(e) // 全字段可编码类型，不会失败
	return out
}

func decodeSimilarityEdge(raw []byte) (SimilarityEdge, bool) {
	var e SimilarityEdge
	if err := json.Unmarshal(raw, &e); err != nil {
		return SimilarityEdge{}, false
	}
	return e, true
}

// Package schema 提供「按数据类型注册校验规则」的最小注册表：每种业务数据类型
// （如行情快照）注册一个 Validator，落盘前按 key 前缀分派到对应校验器。
//
// M1（契约层，docs/rfc/Kair-2.md §9 Schema Descriptor）起，注册表存的是 Descriptor
// （类型的全部声明式语义：key 布局、单调性策略、必填字段、量纲、幂等键、PIT 语义、
// 生产者/消费者、schema 版本），Validator 是 Descriptor 的一个字段。
//
// 定位：这一层只管「一条记录内容是否合法、类型声明哪些时间语义」，不管帧格式
// （畸形帧/超限由 service/ingesthook.Filter 处理）也不管传输层（kairnet 裸帧 / gRPC
// 结构体皆可复用，见 Filter.Validate 的调用方）。
//
// 分派方式：v2 有线上 `type` 字段的写入按 TypeID 直接查表（精确，见 LookupByType）；
// 没有 `type` 概念的 v1/gRPC 写入，以及 v2 `type=0`（未声明）的写入，仍按 key 前缀做
// 最长匹配（LookupDescriptor）。不支持的 key（无前缀匹配、无 TypeID 匹配）视为
// 「未纳管类型」，不做 schema 校验、直接放行——新增数据类型前，未注册的旧类型不会被
// 误伤。
package schema

import (
	"bytes"
	"sync"
)

// Validator 校验一条记录的 value 字节内容。返回 nil 表示通过；非 nil 时调用方
// 应丢弃该记录，error 内容即丢弃原因（供 metrics/日志使用）。返回的 error 若是
// *ValidationError，携带机读错误码（见 ValidationError 与 Err* 常量）；否则调用方
// 应退回到通用的默认码。
type Validator interface {
	Validate(value []byte) error
}

// TimeKind 声明类型的时间语义；当前只接受 none，非 none 在加载时拒绝。
type TimeKind int

const (
	// TimeKindNone 表示这个类型不需要单调性检查（如 quote：日频快照允许乱序/重复）。
	TimeKindNone TimeKind = iota
	// TimeKindStrictlyIncreasing 表示 KeyField 对应的字段必须严格递增。
	// M1 尚未实现这一档的实际检查逻辑（见 LoadContracts 对非 none 取值的拒绝），
	// 声明它是为了让类型自己表达意图，而不是无法表达。
	TimeKindStrictlyIncreasing
	// TimeKindNonDecreasing 同上，允许相等（非递减）。
	TimeKindNonDecreasing
)

// KeyLayout 用分隔符 + 字段名列表声明 key 形状，替代"包注释里写一句人话"
// （Kair-2 RFC §9.2）。例：quote 类型 = {Delimiter: ":", Fields: ["prefix","date","code"]}。
type KeyLayout struct {
	Delimiter string
	Fields    []string
}

// TimeSemantics 声明这个类型是否需要单调性保证，以及哪个 KeyLayout 字段扮演
// "时间戳"角色。Kind==TimeKindNone 时 KeyField 无意义。
type TimeSemantics struct {
	Kind     TimeKind
	KeyField string
}

// Descriptor 是一个数据类型的全部声明式语义（Kair-2 RFC §9.2 的落地）：
// key 布局、单调性策略、校验规则、量纲、幂等键、PIT 语义、生产者/消费者、schema
// 版本，一个类型只在一个地方声明，而不是散落在校验代码、过滤器旁路、文档注释、
// 建表 DDL 注释里各写一份。
type Descriptor struct {
	// TypeID 对应 Kair v2 协议帧里的 type 字段（kairnet/codec.HeaderV2.Type）。
	// 0 表示未分配 TypeID（只能靠 KeyPrefix 做 v1/gRPC 场景的前缀匹配，见 RegisterDescriptor）。
	TypeID uint16
	Name   string

	// SchemaVersion 是这个类型的契约版本号，供"字段何时引入/废弃"这类演进可追溯
	// （方案 §2.4 风险 2：schema 演化不应只靠文件名 + 注释）。
	SchemaVersion int

	// KeyPrefix 是 v1/gRPC 场景下按前缀最长匹配用的字面量前缀（如 "quote:"）。
	// 空字符串表示这个类型不参与前缀匹配（只能靠 TypeID 精确查找）。
	KeyPrefix string
	KeyLayout KeyLayout

	TimeSemantics TimeSemantics

	RequiredFields []string
	Units          map[string]string
	IdempotencyKey []string
	PITSemantics   string
	Producer       string
	Consumers      []string

	Validation Validator
}

var (
	mu sync.RWMutex

	// prefixRegistry 按 KeyPrefix 索引 Descriptor。
	prefixRegistry = map[string]Descriptor{}
	// typeRegistry 按 TypeID 索引，供 v2 `type` 字段精确分派（LookupByType）。只有
	// TypeID != 0 的 Descriptor 才会出现在这里。
	typeRegistry = map[uint16]Descriptor{}
)

// RegisterDescriptor 注册一个完整的类型 Descriptor：写入 KeyPrefix 索引（若非空）
// 与 TypeID 索引（若非 0）。重复注册同一 KeyPrefix/TypeID 会覆盖旧的。
func RegisterDescriptor(d Descriptor) {
	mu.Lock()
	defer mu.Unlock()
	if d.KeyPrefix != "" {
		prefixRegistry[d.KeyPrefix] = d
	}
	if d.TypeID != 0 {
		typeRegistry[d.TypeID] = d
	}
}

// Unregister 移除 prefix 对应的 Descriptor（含其 TypeID 索引，若有），主要供测试
// 清理注册表用。
func Unregister(prefix string) {
	mu.Lock()
	defer mu.Unlock()
	if d, ok := prefixRegistry[prefix]; ok && d.TypeID != 0 {
		delete(typeRegistry, d.TypeID)
	}
	delete(prefixRegistry, prefix)
}

// LookupDescriptor 按 key 前缀做最长匹配，返回完整 Descriptor。
func LookupDescriptor(key []byte) (Descriptor, bool) {
	mu.RLock()
	defer mu.RUnlock()

	var best Descriptor
	bestLen := -1
	for prefix, d := range prefixRegistry {
		if len(prefix) <= bestLen {
			continue
		}
		if bytes.HasPrefix(key, []byte(prefix)) {
			best = d
			bestLen = len(prefix)
		}
	}
	if bestLen < 0 {
		return Descriptor{}, false
	}
	return best, true
}

// LookupByType 按 v2 协议帧的 TypeID 精确查找 Descriptor（Kair-2 RFC §3.2/§9：
// "分派规则从按 key 前缀最长匹配改为按 type 字段直接查表"）。TypeID==0（未声明
// 类型）不应调用本函数——调用方应退回 LookupDescriptor 的前缀匹配路径。
func LookupByType(typeID uint16) (Descriptor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := typeRegistry[typeID]
	return d, ok
}

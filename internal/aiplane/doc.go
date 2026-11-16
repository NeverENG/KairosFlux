// Package aiplane 保存并查询研究提议、相似度记录和证据关系。
// Context 按 as_of 组装研究数据；因子计算、相似性判断和研究推进由调用方负责。
// 证据链查询沿 factor -> experiment -> strategy -> paper -> review 前缀逐跳读取，
// 仅组装已记录的事实，不替上游判断是否采纳因子或进入实盘。
//
// 依赖关系：本包依赖 internal/researchdata（复用其 Store 接口形状与 V2Store 的
// GetAsOf/ListPrefix 能力）与 internal/temporal（Fingerprint 风格一致的
// 确定性指纹方法）。不新增协议 opcode——Proposal/FactorSimilarity/证据边
// 全部走既有 PUT_VERSIONED/GET_AS_OF/LIST_WRITES opcode 上的新键空间前缀
// （proposal:/factor:similarity:/evidence:），协议层不需要注册新 schema
// type_id 也能工作（TypeUnspecified 未纳管前缀默认放行，见
// service/ingesthook/filter.go 的 validate 回退路径）。
//
// 已知边界（曾经写进"发现的歧义"，现状更新如下）：
//   - [已解决] WriteAsAgent 此前只是本包 API 层的一道闸门：任何调用方可以
//     绕过它直接调用 Writer.PutVersioned 写任意键。现在 service/router_v2.go
//     的 handlePutVersioned 会按 PUT_VERSIONED 请求帧已解出的 source 字段
//     独立地重新校验一遍同一条规则（internal/identity.SourceRole 判定为
//     RoleAgent 时，internal/identity.IsProposalKey 判定 key 不是 Proposal
//     对象则结构化拒绝，错误码 codec.ErrCodeUnauthorizedRole），不再依赖
//     调用方"有没有走 WriteAsAgent"这条自觉。
//   - [仍是边界] 上述协议层强制只覆盖 PUT_VERSIONED opcode（自带 source
//     字段）。字面量 PUT/DEL（v1 opcode、v2 OpcodePut/OpcodeDel）不携带
//     source，不受这条角色规则约束——它们从设计上就不是"写某个逻辑键的一条
//     版本化记录"，与 Proposal/Strategy 等对象模型不是同一套键空间语义，
//     本次任务不把这条规则强行套到不携带角色信息的协议路径上。
//   - `strategy:index:{fingerprint}`（M3 registry 导入落的键）实际存的是
//     QuantBrew 的实验/verdict 记录，不是方案 §3.2 对象模型表里定义的
//     "Strategy 策略库存档"对象——命名与内容不对齐，是发现的既有歧义，本包
//     按"它就是 Evidence 图谱里的 Experiment 节点"来读，不改名、不改写。
package aiplane

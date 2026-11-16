package jobctl

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ChronoBrew/KairosFlux/kairnet/codec"
	"github.com/ChronoBrew/KairosFlux/kairnet/negotiate"
	"github.com/ChronoBrew/KairosFlux/proto"
)

// V2Store is a reusable v2 client for versioned research data and as-of reads.
type V2Store struct {
	addr    string
	timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
}

// NewV2Store 构造一个尚未连接的 V2Store；第一次调用 PutVersioned/GetLatest
// 时才真正拨号（懒连接），连接失败或读写出错后下一次调用会重新拨号。
func NewV2Store(addr string, timeout time.Duration) *V2Store {
	return &V2Store{addr: addr, timeout: timeout}
}

func (s *V2Store) ensureConn() (net.Conn, error) {
	if s.conn != nil {
		return s.conn, nil
	}
	conn, err := net.DialTimeout("tcp", s.addr, s.timeout)
	if err != nil {
		return nil, fmt.Errorf("拨号 %s 失败: %w", s.addr, err)
	}
	version, ack, err := negotiate.ClientNegotiateWithAck(conn, s.timeout, negotiate.AckEvery)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("v2 协商失败: %w", err)
	}
	if version != negotiate.VersionV2 {
		conn.Close()
		return nil, fmt.Errorf("服务端不支持 v2 协议（协商结果=%v）", version)
	}
	if ack != negotiate.AckEvery {
		conn.Close()
		return nil, fmt.Errorf("服务端确认 ack 档位=%v，期望 every", ack)
	}
	s.conn = conn
	return conn, nil
}

// roundTrip serializes requests on one connection and drops it after a transport error.
func (s *V2Store) roundTrip(opcode uint8, payload []byte) (*codec.MessageV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, err := s.ensureConn()
	if err != nil {
		return nil, err
	}

	msg := codec.NewMessageV2(codec.HeaderV2{Opcode: opcode, Type: codec.TypeUnspecified, CorrID: 1}, payload)
	frame, err := codec.NewDataPackV2().Pack(msg)
	if err != nil {
		return nil, fmt.Errorf("编帧失败: %w", err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
		s.dropConn()
		return nil, fmt.Errorf("设置写超时失败: %w", err)
	}
	if _, err := conn.Write(frame); err != nil {
		s.dropConn()
		return nil, fmt.Errorf("写帧失败: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(s.timeout)); err != nil {
		s.dropConn()
		return nil, fmt.Errorf("设置读超时失败: %w", err)
	}
	resp, err := codec.NewDataPackV2().Decode(conn, 0, nil)
	if err != nil {
		s.dropConn()
		return nil, fmt.Errorf("读帧失败: %w", err)
	}
	return resp, nil
}

// dropConn 必须在已持有 s.mu 时调用。
func (s *V2Store) dropConn() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

func errFromErrResp(resp *codec.MessageV2) error {
	code, reason, ok := proto.DecodeV2ErrPayload(resp.Payload)
	if !ok {
		return fmt.Errorf("服务端返回 ERR，但负载无法解析")
	}
	return fmt.Errorf("服务端拒绝: code=%#x reason=%s", code, reason)
}

// PutVersioned 实现 Store：对应 PUT_VERSIONED opcode。source 编进请求帧的
// M2 操作元数据信封（proto.EncodePutVersionedFrame），落到服务端账本上供
// LIST_WRITES 审计按来源聚合——此前本方法一直用 proto.EncodePutFrame（不带
// source 段），源头审计缺口正是本次任务要补的（见 store.go EngineSource
// 文档）。
func (s *V2Store) PutVersioned(logicalKey string, payload []byte, source string) (uint64, error) {
	resp, err := s.roundTrip(codec.OpcodePutVersioned, proto.EncodePutVersionedFrame([]byte(logicalKey), payload, source))
	if err != nil {
		return 0, err
	}
	if resp.Header.Opcode != codec.OpcodeOK {
		return 0, errFromErrResp(resp)
	}
	if len(resp.Payload) != 8 {
		return 0, fmt.Errorf("OK 响应负载长度=%d，期望 8（seq）", len(resp.Payload))
	}
	return binary.LittleEndian.Uint64(resp.Payload), nil
}

// GetLatest 实现 Store：对应 GET_AS_OF opcode，as_of 取调用时刻——PUT_VERSIONED
// 保证不会有"未来"写入，as_of=now 恒等于"当前最新版本"（GetAsOf 语义见
// internal/temporal.AsOf 文档），这就是本包不新增"GET_CURRENT"之类的 opcode
// 也能拿到当前值的原因。
func (s *V2Store) GetLatest(logicalKey string) ([]byte, bool, error) {
	resp, err := s.roundTrip(codec.OpcodeGetAsOf, proto.EncodeAsOfFrame([]byte(logicalKey), time.Now().UnixNano()))
	if err != nil {
		return nil, false, err
	}
	if resp.Header.Opcode != codec.OpcodeOK {
		if _, reason, ok := proto.DecodeV2ErrPayload(resp.Payload); ok && reason == "notfound" {
			return nil, false, nil
		}
		return nil, false, errFromErrResp(resp)
	}
	_, _, payload, _, ok := proto.DecodeVersionEntry(resp.Payload)
	if !ok {
		return nil, false, fmt.Errorf("响应负载无法解析")
	}
	return payload, true, nil
}

// GetAsOf reads the version visible at an explicitly chosen write timestamp.
func (s *V2Store) GetAsOf(logicalKey string, asOfNanos int64) ([]byte, bool, error) {
	resp, err := s.roundTrip(codec.OpcodeGetAsOf, proto.EncodeAsOfFrame([]byte(logicalKey), asOfNanos))
	if err != nil {
		return nil, false, err
	}
	if resp.Header.Opcode != codec.OpcodeOK {
		if _, reason, ok := proto.DecodeV2ErrPayload(resp.Payload); ok && reason == "notfound" {
			return nil, false, nil
		}
		return nil, false, errFromErrResp(resp)
	}
	_, _, payload, _, ok := proto.DecodeVersionEntry(resp.Payload)
	if !ok {
		return nil, false, fmt.Errorf("响应负载无法解析")
	}
	return payload, true, nil
}

// ListPrefix 实现 M4 证据图谱查询依赖的前缀扫描：对既有 LIST_WRITES opcode
// （0x0D，M2 时态内核审计查询新增，见 proto.EncodeListWritesRequest 文档）
// 发起请求，asOfNanos 作为写入时间上界（tToNanos）——不新增 opcode，"BTree
// 前缀扫描"直接复用 service.TemporalStore.ListWrites 已经实现的
// [prefix, prefix+0xFF) 范围扫描。返回的是"该前缀下每个逻辑键的每一条历史
// 版本"（与 LIST_WRITES 审计语义一致），调用方（internal/aiplane）自己按
// (LogicalKey, Seq) 取最大值折叠到"每个逻辑键的最新版本"——折叠逻辑不下沉
// 到这里，是因为 aiplane 包的 fakePrefixStore 测试替身需要产生与生产
// 完全一致的折叠前语义，折叠只应该有一处实现（aiplane.LatestPerLogicalKey）。
func (s *V2Store) ListPrefix(prefix string, asOfNanos int64) ([]proto.WriteEnvelopeView, error) {
	resp, err := s.roundTrip(codec.OpcodeListWrites, proto.EncodeListWritesRequest([]byte(prefix), 0, asOfNanos, nil))
	if err != nil {
		return nil, err
	}
	if resp.Header.Opcode != codec.OpcodeOK {
		return nil, errFromErrResp(resp)
	}
	entries, _, ok := proto.DecodeListWritesResponse(resp.Payload)
	if !ok {
		return nil, fmt.Errorf("响应负载无法解析")
	}
	return entries, nil
}

// Close 关闭底层连接（进程退出前清理）。
func (s *V2Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

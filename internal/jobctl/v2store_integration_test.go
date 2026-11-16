package jobctl

// V2Store integration test runs a real v2 server for versioned registry imports.
//
// 服务端起法复制自 service/router_v2_integration_test.go 的
// startRouterV2TestServer（本包在 service 包之外，用它导出的
// NewKVServer/NewRouter/NewRouterV2 等构造函数自己拼一遍，不能直接调用那
// 个测试专用的非导出函数）。

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ChronoBrew/KairosFlux/config"
	"github.com/ChronoBrew/KairosFlux/kairnet"
	"github.com/ChronoBrew/KairosFlux/proto"
	"github.com/ChronoBrew/KairosFlux/service"
	"github.com/ChronoBrew/KairosFlux/service/ingesthook"
)

func startTestKairosFluxServer(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	oldWAL, oldSST, oldMode := config.G.WALPath, config.G.SSTablePath, config.G.Mode
	config.G.WALPath = filepath.Join(dir, "wal.log")
	config.G.SSTablePath = dir
	config.G.Mode = config.ModeStandalone
	t.Cleanup(func() {
		config.G.WALPath, config.G.SSTablePath, config.G.Mode = oldWAL, oldSST, oldMode
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("取空闲端口失败: %v", err)
	}
	addr := l.Addr().String()
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	l.Close()

	kv := service.NewKVServer()
	router := service.NewRouter(kv)
	filter := ingesthook.NewFilter(nil, 0, false)
	router.SetPreHandle(filter.Handle)
	routerV2 := service.NewRouterV2(kv, filter, service.DefaultV2WindowSafetyValveN)

	srv := kairnet.NewServer()
	srv.IP = host
	srv.Port = port
	srv.AddRouter(proto.MsgPut, router)
	srv.AddRouter(proto.MsgGet, router)
	srv.AddRouter(proto.MsgDelete, router)
	srv.AddRouter(proto.MsgScan, router)
	srv.AddRouterV2(routerV2)
	srv.SetConnStartFunc(router.OnConnStart)
	srv.SetConnStopFunc(router.OnConnStop)
	srv.Start()
	t.Cleanup(func() { srv.Stop(); kv.Close() })

	for i := 0; i < 100; i++ {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return addr
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("服务端在 2s 内未就绪: %s", addr)
	return ""
}

func TestV2Store_PutVersionedThenGetLatestRoundTrip(t *testing.T) {
	addr := startTestKairosFluxServer(t)
	store := NewV2Store(addr, 3*time.Second)
	defer store.Close()

	seq, err := store.PutVersioned("strategy:index:integration_experiment", []byte(`{"phase":"succeeded"}`), EngineSource)
	if err != nil {
		t.Fatalf("PutVersioned 失败: %v", err)
	}
	if seq != 1 {
		t.Fatalf("首次写入 seq 应为 1，实际 %d", seq)
	}

	payload, found, err := store.GetLatest("strategy:index:integration_experiment")
	if err != nil {
		t.Fatalf("GetLatest 失败: %v", err)
	}
	if !found {
		t.Fatal("刚写入的键应能读到")
	}
	if string(payload) != `{"phase":"succeeded"}` {
		t.Fatalf("读回内容不符: %s", payload)
	}

	_, found, err = store.GetLatest("strategy:index:never_written")
	if err != nil {
		t.Fatalf("GetLatest 对未写入键出错: %v", err)
	}
	if found {
		t.Fatal("未写入的键应返回 found=false")
	}
}

// A real import must create the index consumed by the AI research context.
func TestRegistryImporterOverV2Store(t *testing.T) {
	addr := startTestKairosFluxServer(t)
	store := NewV2Store(addr, 3*time.Second)
	defer store.Close()
	input := `{"verdict":{"fingerprint":"integration-001","level":"pass_raw"}}` + "\n"
	imp := &RegistryImporter{Store: store}
	for i := 0; i < 2; i++ {
		result, err := imp.ImportReader(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && result.Imported != 1 || i == 1 && result.Unchanged != 1 {
			t.Fatalf("import %d: %+v", i, result)
		}
	}
	raw, found, err := store.GetLatest(RegistryIndexKey("integration-001"))
	if err != nil || !found || string(raw) != strings.TrimSpace(input) {
		t.Fatalf("index read: found=%t raw=%s err=%v", found, raw, err)
	}
}

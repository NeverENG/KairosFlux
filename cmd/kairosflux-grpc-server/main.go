// 命令 kairosflux-grpc-server 是 Kair(kairnet TLV) 协议的基准测试/协议对照服务端，
// 不是生产摄入入口——生产摄入用 cmd/kairosflux-server（Kair）。实测显示 Kair 在
// 不受 fsync 约束的读路径上吞吐约为 gRPC 的 2.7 倍（写路径两者持平，均受同一
// fsync 瓶颈约束），这是自研协议存在的理由；本命令的作用是给 Kair 提供一个
// 可对照的性能与协议基线，见 internal/kvgrpc 包注释、README.md 的性能一节与
// docs/Kair-协议规范.md。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ChronoBrew/KairosFlux/internal/kvgrpc"
	"github.com/ChronoBrew/KairosFlux/service"
	"github.com/ChronoBrew/KairosFlux/service/ingesthook"
)

func main() {
	addr := flag.String("addr", "localhost:9090", "gRPC server listen address")
	flag.Parse()

	kvServer := service.NewKVServer()

	go kvServer.Run()

	ha := service.NewHA(kvServer)

	// 挂载与 Kair 入口相同的长度与 schema 校验。
	filter := ingesthook.NewFilter(0)
	grpcSrv := kvgrpc.NewGRPCServer(kvServer, filter)

	fmt.Println("Starting gRPC Server...")
	fmt.Printf("HA initialized, initial health status: %v\n", ha.IsHealthy())
	fmt.Printf("Listening on %s\n", *addr)

	if err := grpcSrv.Serve(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "gRPC server error: %v\n", err)
		os.Exit(1)
	}
}

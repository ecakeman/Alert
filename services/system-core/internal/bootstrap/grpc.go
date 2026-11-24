package bootstrap

import (
	"context"
	"net"
	"time"

	googlegrpc "google.golang.org/grpc"

	transportgrpc "github.com/ecakeman/alert/services/system-core/internal/transport/grpc"
)

// GRPCServer 承载 api 角色的回调服务端生命周期。
type GRPCServer struct {
	server *googlegrpc.Server
	lis    net.Listener
}

func NewGRPCServer(sh *Shared, ucs *UseCases) (*GRPCServer, error) {
	lis, err := net.Listen("tcp", sh.Cfg.GRPCAddr)
	if err != nil {
		return nil, err
	}
	server := googlegrpc.NewServer(googlegrpc.UnaryInterceptor(transportgrpc.UnaryInterceptor()))
	transportgrpc.Register(server, transportgrpc.NewServer(ucs.Task))
	return &GRPCServer{server: server, lis: lis}, nil
}

func (g *GRPCServer) Serve() error { return g.server.Serve(g.lis) }

func (g *GRPCServer) GracefulStop(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		g.server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		g.server.Stop()
	}
}

func (g *GRPCServer) Addr() net.Addr { return g.lis.Addr() }

var _ = time.Second

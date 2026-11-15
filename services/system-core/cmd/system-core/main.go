package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	agentv1 "github.com/ecakeman/alertflow/services/system-core/gen/agent/v1"
	systemv1 "github.com/ecakeman/alertflow/services/system-core/gen/system/v1"
	"github.com/ecakeman/alertflow/services/system-core/internal/api"
	"github.com/ecakeman/alertflow/services/system-core/internal/cache"
	"github.com/ecakeman/alertflow/services/system-core/internal/db"
	"github.com/ecakeman/alertflow/services/system-core/internal/gateway"
	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring"
	"github.com/ecakeman/alertflow/services/system-core/internal/monitoring/probe/kubernetes"
	"github.com/ecakeman/alertflow/services/system-core/internal/system"
	"github.com/ecakeman/alertflow/services/system-core/internal/transport/grpc/systemapi"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "health" {
		os.Exit(healthCheck())
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL := env("ALERTFLOW_DATABASE_URL", "postgres://alertflow:alertflow@127.0.0.1:5432/alertflow?sslmode=disable")
	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer conn.Close()
	store := system.New(conn)
	var aside cache.Aside
	if redisURL := os.Getenv("ALERTFLOW_REDIS_URL"); redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Error("redis", "err", err)
			os.Exit(1)
		}
		aside.Client = redis.NewClient(opt)
		aside.TTL = 5 * time.Second
	}
	root, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Kubernetes 观察器默认关闭；启用但集群不可用时只记录错误，采样返回观察器错误。
	k8sEnabled, parseErr := strconv.ParseBool(env("ALERTFLOW_K8S_ENABLED", "false"))
	if parseErr != nil {
		log.Warn("invalid ALERTFLOW_K8S_ENABLED, kubernetes monitoring disabled", "value", os.Getenv("ALERTFLOW_K8S_ENABLED"))
		k8sEnabled = false
	}
	k8sObserver := kubernetes.NewK8sObserver(k8sEnabled, env("ALERTFLOW_K8S_NAMESPACE", "default"), log)
	go monitoring.Run(root, store, k8sObserver, log)
	go gateway.RunRecovery(root, store, 2*time.Second)
	if brokers := split(os.Getenv("ALERTFLOW_KAFKA_BROKERS")); len(brokers) > 0 && brokers[0] != "" {
		topic := env("ALERTFLOW_KAFKA_TOPIC", "alertflow.agent-task-ready.v1")
		go gateway.RunPublisher(root, store, brokers, topic, log)
		if target := os.Getenv("ALERTFLOW_AGENT_GRPC_TARGET"); target != "" {
			go gateway.RunConsumer(root, store, brokers, topic, "alertflow-agent-gateway", target, log)
		}
	}
	grpcServer := grpc.NewServer()
	apiServer := &systemapi.Server{API: &api.Server{Store: store, Cache: aside, Log: log, Artifacts: agentArtifacts(os.Getenv("ALERTFLOW_AGENT_GRPC_TARGET"))}}
	systemv1.RegisterSystemApiServer(grpcServer, apiServer)
	grpc_health_v1.RegisterHealthServer(grpcServer, apiServer)
	agentv1.RegisterSystemGatewayServiceServer(grpcServer, &gateway.SystemServer{Store: store})
	lis, err := net.Listen("tcp", env("ALERTFLOW_GRPC_ADDR", ":9090"))
	if err != nil {
		log.Error("grpc listen", "err", err)
		os.Exit(1)
	}
	const shutdownTimeout = 10 * time.Second
	go func() {
		<-root.Done()
		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			grpcServer.Stop()
		}
	}()
	log.Info("system-core listening", "grpc", lis.Addr().String())
	if err := grpcServer.Serve(lis); err != nil {
		log.Error("grpc", "err", err)
		os.Exit(1)
	}
}

func healthCheck() int {
	conn, err := grpc.NewClient("127.0.0.1:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 1
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return 1
	}
	return 0
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func split(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

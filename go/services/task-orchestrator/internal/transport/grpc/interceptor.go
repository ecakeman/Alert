package grpc

import (
	"context"
	"log"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const deadlineCap = 2 * time.Second

// UnaryInterceptor 提供 RequestID、访问日志、panic 恢复与 deadline 上限。
func UnaryInterceptor() googlegrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *googlegrpc.UnaryServerInfo, handler googlegrpc.UnaryHandler) (any, error) {
		start := time.Now()
		requestID := requestIDFrom(ctx)
		resp, err := invokeWithRecovery(ctx, req, handler)
		log.Printf("grpc method=%s request_id=%s code=%s latency=%s",
			info.FullMethod, requestID, status.Code(err).String(), time.Since(start))
		return resp, err
	}
}

func invokeWithRecovery(ctx context.Context, req any, handler googlegrpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("grpc panic recovered: %v", recovered)
			err = status.Error(codes.Internal, "INTERNAL: panic recovered")
		}
	}()
	ctx, cancel := withDeadlineCap(ctx)
	defer cancel()
	return handler(ctx, req)
}

func withDeadlineCap(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > deadlineCap {
		return context.WithTimeout(ctx, deadlineCap)
	}
	return ctx, func() {}
}

func requestIDFrom(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-request-id")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

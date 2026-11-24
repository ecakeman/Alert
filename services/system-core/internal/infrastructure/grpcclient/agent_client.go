package grpcclient

import (
	"context"
	"encoding/base64"
	"strconv"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	portdispatch "github.com/ecakeman/alert/services/system-core/internal/application/port/dispatch"
	porttask "github.com/ecakeman/alert/services/system-core/internal/application/port/task"
	agentv1 "github.com/ecakeman/alert/services/system-core/internal/transport/grpc/gen/alertflow/agent/v1"
	systemv1 "github.com/ecakeman/alert/services/system-core/internal/transport/grpc/gen/alertflow/system/v1"
)

// AgentClient 实现 TaskSubmitter；timeout 由 AGENT_GRPC_TIMEOUT 注入。
type AgentClient struct {
	conn    *googlegrpc.ClientConn
	client  agentv1.AgentTaskServiceClient
	timeout time.Duration
}

var _ portdispatch.TaskSubmitter = (*AgentClient)(nil)

// NewAgentClient 以 WithBlock 建立连接；连接关闭由 worker 生命周期负责。
func NewAgentClient(ctx context.Context, target string, timeout time.Duration) (*AgentClient, error) {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := googlegrpc.DialContext(dialCtx, target,
		googlegrpc.WithBlock(),
		googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &AgentClient{conn: conn, client: agentv1.NewAgentTaskServiceClient(conn), timeout: timeout}, nil
}

// SubmitTask 返回 accepted 仅表示协议级受理；业务拒绝不回退重试。
func (c *AgentClient) SubmitTask(ctx context.Context, task porttask.AgentTask) (bool, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	leaseUntil := int64(0)
	if task.LeaseUntil != nil {
		leaseUntil = task.LeaseUntil.Unix()
	}
	resp, err := c.client.SubmitTask(callCtx, &agentv1.SubmitTaskRequest{
		Identity: &systemv1.TaskIdentity{
			TaskId:     task.TaskID,
			EpisodeId:  task.EpisodeID,
			TaskType:   task.TaskType,
			Generation: task.Generation,
			LeaseToken: encodeLeaseToken(task.TaskID, task.Generation, leaseUntil),
		},
		ServiceId:         task.ServiceID,
		InputVersionFrom:  task.InputVersionFrom,
		InputVersionTo:    task.InputVersionTo,
		MonitorWatermark:  task.MonitorWatermark,
		AppEventWatermark: task.AppEventWatermark,
		DeadlineAt:        task.DeadlineAt.Unix(),
	})
	if err != nil {
		switch status.Code(err) {
		case codes.Unavailable, codes.DeadlineExceeded:
			return false, err
		default:
			return false, nil
		}
	}
	return resp.GetAccepted(), nil
}

func (c *AgentClient) Close() error { return c.conn.Close() }

func encodeLeaseToken(taskID string, generation, leaseUntilUnix int64) string {
	raw := taskID + "|" + strconv.FormatInt(generation, 10) + "|" + strconv.FormatInt(leaseUntilUnix, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

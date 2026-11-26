package grpcclient

import (
	"context"
	"encoding/base64"
	"strconv"
	"sync"
	"time"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	portdispatch "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/dispatch"
	porttask "github.com/ecakeman/alert/services/task-orchestrator/internal/application/port/task"
	agentv1 "github.com/ecakeman/alert/services/task-orchestrator/internal/transport/grpc/gen/alertflow/agent/v1"
	systemv1 "github.com/ecakeman/alert/services/task-orchestrator/internal/transport/grpc/gen/alertflow/system/v1"
)

// AgentClient 实现 TaskSubmitter；惰性建连，避免认知栈未起时阻塞启动。
type AgentClient struct {
	target  string
	timeout time.Duration
	mu      sync.Mutex
	conn    *googlegrpc.ClientConn
	client  agentv1.AgentTaskServiceClient
}

var _ portdispatch.TaskSubmitter = (*AgentClient)(nil)

func NewAgentClient(target string, timeout time.Duration) *AgentClient {
	return &AgentClient{target: target, timeout: timeout}
}

func (c *AgentClient) ensureClient() (agentv1.AgentTaskServiceClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	conn, err := googlegrpc.NewClient(c.target, googlegrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	c.conn = conn
	c.client = agentv1.NewAgentTaskServiceClient(conn)
	return c.client, nil
}

// SubmitTask 返回 accepted 仅表示协议级受理；业务拒绝不回退重试。
func (c *AgentClient) SubmitTask(ctx context.Context, task porttask.AgentTask) (bool, error) {
	client, err := c.ensureClient()
	if err != nil {
		return false, err
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	leaseUntil := int64(0)
	if task.LeaseUntil != nil {
		leaseUntil = task.LeaseUntil.Unix()
	}
	resp, err := client.SubmitTask(callCtx, &agentv1.SubmitTaskRequest{
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

func (c *AgentClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func encodeLeaseToken(taskID string, generation, leaseUntilUnix int64) string {
	raw := taskID + "|" + strconv.FormatInt(generation, 10) + "|" + strconv.FormatInt(leaseUntilUnix, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
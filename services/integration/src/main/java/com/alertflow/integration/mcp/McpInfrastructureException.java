package com.alertflow.integration.mcp;

// 基础设施不可用（策略库/DB 等）：HTTP 边界映射为 500，不得伪装成 401 或“无权限”。
public class McpInfrastructureException extends RuntimeException {
    public McpInfrastructureException(String message, Throwable cause) {
        super(message, cause);
    }
}

package com.alertflow.integration.mcp;

// 认证失败：HTTP 边界映射为 401，不与其他业务错误混淆。
public class McpAuthException extends IllegalStateException {
    public McpAuthException(String message) {
        super(message);
    }
}

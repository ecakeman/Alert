package com.alertflow.integration.mcp;

// 请求格式错误（JSON 解析失败、JSON-RPC 结构非法）：HTTP 边界映射为 400。
public class McpBadRequestException extends RuntimeException {
    public McpBadRequestException(String message) {
        super(message);
    }
}

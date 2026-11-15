package com.alertflow.integration.mcp;

import java.util.List;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RestController;

// MCP 的同步 HTTP 入口：Spring MVC + 虚拟线程，网关本身只做同步 JSON-RPC 处理。
@RestController
public class McpController {
    private static final Logger log = LoggerFactory.getLogger(McpController.class);
    private final McpGateway gateway;

    public McpController(McpGateway gateway) {
        this.gateway = gateway;
    }

    @PostMapping(path = "/mcp", produces = MediaType.APPLICATION_JSON_VALUE)
    public ResponseEntity<String> mcp(@RequestHeader(value = "Authorization", required = false) String authorization,
                                      @RequestBody(required = false) String body) {
        try {
            return ResponseEntity.ok()
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(gateway.dispatch(authorization, body));
        } catch (McpAuthException error) {
            return error(HttpStatus.UNAUTHORIZED, "unauthorized");
        } catch (McpBadRequestException error) {
            return error(HttpStatus.BAD_REQUEST, "invalid request");
        } catch (McpInfrastructureException error) {
            log.error("mcp request failed: policy store unavailable", error);
            return error(HttpStatus.INTERNAL_SERVER_ERROR, "internal error");
        } catch (Exception error) {
            log.error("mcp request failed", error);
            return error(HttpStatus.INTERNAL_SERVER_ERROR, "internal error");
        }
    }

    private static ResponseEntity<String> error(HttpStatus status, String message) {
        return ResponseEntity.status(status)
                .contentType(MediaType.APPLICATION_JSON)
                .body("{\"error\":\"" + message + "\"}");
    }

    // 仅用于测试/诊断的只读方法列表，不参与协议。
    public List<String> protocols() {
        return gateway.supported();
    }
}

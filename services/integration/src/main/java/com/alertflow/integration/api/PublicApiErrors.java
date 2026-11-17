package com.alertflow.integration.api;

import java.util.Map;

import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.HttpStatusCode;
import org.springframework.http.MediaType;
import org.springframework.http.ProblemDetail;
import org.springframework.http.ResponseEntity;
import org.springframework.web.ErrorResponse;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.context.request.WebRequest;
import org.springframework.web.servlet.mvc.method.annotation.ResponseEntityExceptionHandler;

/**
 * 把 Spring 自身产生、但业务上属于请求错误的响应（405 方法不允许、404 路由不存在、400 请求体不可读 等）
 * 改写成与 Go 系统侧一致的 {error_code,message} 结构，避免对外同时存在两种错误格式。
 */
@RestControllerAdvice
public class PublicApiErrors extends ResponseEntityExceptionHandler {
    private static final MediaType JSON = MediaType.APPLICATION_JSON;

    @Override
    protected ResponseEntity<Object> handleExceptionInternal(Exception ex, Object body, HttpHeaders headers,
                                                             HttpStatusCode statusCode, WebRequest request) {
        HttpStatus status = HttpStatus.resolve(statusCode.value());
        String code = status == null ? "ERROR" : status.name();
        ProblemDetail detail = ex instanceof ErrorResponse errorResponse ? errorResponse.getBody() : null;
        String message = detail == null ? code : String.valueOf(detail.getDetail());
        return ResponseEntity.status(statusCode).contentType(JSON).headers(headers).body(errorBody(code, message));
    }

    private static Map<String, String> errorBody(String code, String message) {
        return Map.of("error_code", code, "message", message.replace("\"", "'"));
    }
}

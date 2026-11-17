package com.alertflow.integration;

import com.alertflow.integration.api.PublicApiErrors;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

import org.junit.jupiter.api.Test;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.context.annotation.Bean;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;

// Spring 自身产生的 404/405 也必须走统一错误体，避免对外同时存在两种错误格式（方案 §12.2）。
@SpringBootTest(
        classes = PublicApiErrorsTest.ProbeApp.class,
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
                "spring.flyway.enabled=false",
                "spring.datasource.url=jdbc:postgresql://127.0.0.1:1/unused",
                "spring.datasource.hikari.initialization-fail-timeout=-1",
                "spring.datasource.hikari.minimum-idle=0",
                "spring.datasource.hikari.maximum-pool-size=1"
        })
class PublicApiErrorsTest {
    @LocalServerPort
    private int port;

    @Test
    void methodNotAllowedUsesUnifiedErrorBody() throws Exception {
        HttpResponse<String> response = send(HttpRequest.newBuilder(base("/probe")).PUT(noBody()));

        assertEquals(405, response.statusCode());
        assertTrue(response.body().contains("\"error_code\":\"METHOD_NOT_ALLOWED\""), response.body());
        assertTrue(response.body().contains("\"message\":"), response.body());
        assertTrue(response.headers().firstValue("Content-Type").orElse("").contains("application/json"));
    }

    @Test
    void unknownRouteUsesUnifiedErrorBody() throws Exception {
        HttpResponse<String> response = send(HttpRequest.newBuilder(base("/no-such-route")).GET());

        assertEquals(404, response.statusCode());
        assertTrue(response.body().contains("\"error_code\":\"NOT_FOUND\""), response.body());
    }

    @Test
    void readableBodyStillAnsweredWithJson() throws Exception {
        HttpResponse<String> response = send(HttpRequest.newBuilder(base("/probe"))
                .header("Content-Type", MediaType.APPLICATION_JSON_VALUE)
                .POST(HttpRequest.BodyPublishers.ofString("\"ok\"")));

        assertEquals(200, response.statusCode());
        assertEquals("accepted", response.body());
    }

    private URI base(String path) {
        return URI.create("http://127.0.0.1:" + port + path);
    }

    private static HttpResponse<String> send(HttpRequest.Builder builder) throws Exception {
        return HttpClient.newHttpClient().send(builder.build(), HttpResponse.BodyHandlers.ofString());
    }

    private static HttpRequest.BodyPublisher noBody() {
        return HttpRequest.BodyPublishers.noBody();
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration
    static class ProbeApp {
        @Bean
        ProbeController probeController() {
            return new ProbeController();
        }

        @Bean
        PublicApiErrors publicApiErrors() {
            return new PublicApiErrors();
        }
    }

    @RestController
    static class ProbeController {
        @GetMapping("/probe")
        String probe() {
            return "ok";
        }

        @PostMapping("/probe")
        String post(@org.springframework.web.bind.annotation.RequestBody(required = false) byte[] body) {
            return body == null ? "empty" : "accepted";
        }
    }
}

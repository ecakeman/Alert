package com.alertflow.integration;

import static org.junit.jupiter.api.Assertions.assertEquals;

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
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

// 验证 Tomcat 请求线程确实由虚拟线程承载（VT-01）。
@SpringBootTest(
        classes = VirtualThreadWebTest.ProbeApp.class,
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
                "spring.threads.virtual.enabled=true",
                "spring.flyway.enabled=false",
                "spring.datasource.url=jdbc:postgresql://127.0.0.1:1/unused",
                "spring.datasource.hikari.initialization-fail-timeout=-1",
                "spring.datasource.hikari.minimum-idle=0",
                "spring.datasource.hikari.maximum-pool-size=1",
                "management.endpoints.web.exposure.include=health"
        })
class VirtualThreadWebTest {
    @LocalServerPort
    private int port;

    @Test
    void requestRunsOnVirtualThread() throws Exception {
        HttpResponse<String> response = HttpClient.newHttpClient().send(
                HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + "/probe")).GET().build(),
                HttpResponse.BodyHandlers.ofString());
        assertEquals(200, response.statusCode());
        assertEquals("true", response.body());
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration
    static class ProbeApp {
        @Bean
        ProbeController probeController() {
            return new ProbeController();
        }
    }

    @RestController
    static class ProbeController {
        @GetMapping("/probe")
        String probe() {
            return Boolean.toString(Thread.currentThread().isVirtual());
        }
    }
}

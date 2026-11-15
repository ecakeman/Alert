package com.alertflow.integration.api;

import java.nio.charset.StandardCharsets;

import java.util.LinkedHashMap;
import java.util.Map;

import com.alertflow.gen.agent.v1.ArtifactType;
import com.alertflow.gen.agent.v1.GetArtifactRequest;
import com.alertflow.gen.system.v1.JsonCall;
import com.google.protobuf.Struct;
import com.google.protobuf.Value;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.alertflow.gen.system.v1.JsonResult;
import com.alertflow.gen.system.v1.ReadyRequest;
import com.alertflow.integration.bridge.GrpcBridge;
import com.alertflow.integration.cache.RevisionCache;
import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import jakarta.servlet.http.HttpServletRequest;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PatchMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class PublicApiController {
    private static final String ALL = "00000000-0000-0000-0000-000000000000";
    private final GrpcBridge bridge;
    private final RevisionCache cache;

    public PublicApiController(GrpcBridge bridge, RevisionCache cache) {
        this.bridge = bridge;
        this.cache = cache;
    }

    @GetMapping("/readyz")
    public ResponseEntity<String> ready() {
        return call(() -> {
            var ready = bridge.systemApi().ready(ReadyRequest.getDefaultInstance());
            int status = "ok".equals(ready.getStatus()) ? 200 : 503;
            String body = status == 200 ? "{\"status\":\"ok\"}" : "{\"status\":\"unavailable\",\"database\":\"down\"}";
            return ResponseEntity.status(status).contentType(MediaType.APPLICATION_JSON).body(body);
        });
    }

    @GetMapping("/api/v1/dashboard/summary")
    public ResponseEntity<String> dashboard(HttpServletRequest request) {
        return cached("dashboard", ALL, () -> json(bridge.systemApi().dashboard(jsonCall(request, null, ""))));
    }

    @GetMapping("/api/v1/services")
    public ResponseEntity<String> listServices(HttpServletRequest request) {
        return cached("services", ALL, () -> json(bridge.systemApi().listServices(jsonCall(request, null, ""))));
    }

    @PostMapping("/api/v1/services")
    public ResponseEntity<String> createService(HttpServletRequest request, @RequestBody(required = false) byte[] body) {
        return call(() -> json(bridge.systemApi().createService(jsonCall(request, null, body))));
    }

    @GetMapping("/api/v1/services/{serviceId}")
    public ResponseEntity<String> getService(HttpServletRequest request, @PathVariable String serviceId) {
        return cached("service", serviceId, () -> json(bridge.systemApi().getService(jsonCall(request, serviceId, ""))));
    }

    @PatchMapping("/api/v1/services/{serviceId}")
    public ResponseEntity<String> patchService(HttpServletRequest request, @PathVariable String serviceId, @RequestBody(required = false) byte[] body) {
        return call(() -> json(bridge.systemApi().patchService(jsonCall(request, serviceId, body))));
    }

    @DeleteMapping("/api/v1/services/{serviceId}")
    public ResponseEntity<String> deleteService(HttpServletRequest request, @PathVariable String serviceId) {
        return call(() -> json(bridge.systemApi().deleteService(jsonCall(request, serviceId, ""))));
    }

    @GetMapping("/api/v1/services/{serviceId}/targets")
    public ResponseEntity<String> listTargets(HttpServletRequest request, @PathVariable String serviceId) {
        return cached("targets", serviceId, () -> json(bridge.systemApi().listTargets(jsonCall(request, serviceId, ""))));
    }

    @PostMapping("/api/v1/services/{serviceId}/targets")
    public ResponseEntity<String> createTarget(HttpServletRequest request, @PathVariable String serviceId, @RequestBody(required = false) byte[] body) {
        return call(() -> json(bridge.systemApi().createTarget(jsonCall(request, serviceId, body))));
    }

    @PatchMapping("/api/v1/targets/{targetId}")
    public ResponseEntity<String> patchTarget(HttpServletRequest request, @PathVariable String targetId, @RequestBody(required = false) byte[] body) {
        return call(() -> json(bridge.systemApi().patchTarget(jsonCall(request, targetId, body))));
    }

    @DeleteMapping("/api/v1/targets/{targetId}")
    public ResponseEntity<String> deleteTarget(HttpServletRequest request, @PathVariable String targetId) {
        return call(() -> json(bridge.systemApi().deleteTarget(jsonCall(request, targetId, ""))));
    }

    @GetMapping("/api/v1/episodes")
    public ResponseEntity<String> listEpisodes(HttpServletRequest request) {
        return cached("episodes", ALL, () -> json(bridge.systemApi().listEpisodes(jsonCall(request, null, ""))));
    }

    @GetMapping("/api/v1/services/{serviceId}/timeline")
    public ResponseEntity<String> serviceTimeline(HttpServletRequest request, @PathVariable String serviceId) {
        return call(() -> json(bridge.systemApi().serviceTimeline(jsonCall(request, serviceId, ""))));
    }

    @PostMapping("/api/v1/services/{serviceId}/events")
    public ResponseEntity<String> postEvent(HttpServletRequest request, @PathVariable String serviceId, @RequestBody(required = false) byte[] body) {
        return call(() -> json(bridge.systemApi().postEvent(jsonCall(request, serviceId, body))));
    }

    @GetMapping("/api/v1/episodes/{episodeId}")
    public ResponseEntity<String> getEpisode(HttpServletRequest request, @PathVariable String episodeId) {
        return call(() -> json(bridge.systemApi().getEpisode(jsonCall(request, episodeId, ""))));
    }

    @GetMapping("/api/v1/episodes/{episodeId}/timeline")
    public ResponseEntity<String> episodeTimeline(HttpServletRequest request, @PathVariable String episodeId) {
        return call(() -> json(bridge.systemApi().episodeTimeline(jsonCall(request, episodeId, ""))));
    }

    @GetMapping("/api/v1/episodes/{episodeId}/artifacts")
    public ResponseEntity<String> artifacts(HttpServletRequest request, @PathVariable String episodeId) {
        ArtifactType kind = "DIAGNOSIS_REPORT".equalsIgnoreCase(request.getParameter("type"))
                ? ArtifactType.ARTIFACT_TYPE_DIAGNOSIS_REPORT
                : ArtifactType.ARTIFACT_TYPE_INVESTIGATION;
        try {
            return bridge.callPython(() -> {
                var response = bridge.pythonExecution().getArtifact(GetArtifactRequest.newBuilder()
                        .setEpisodeId(episodeId)
                        .setArtifactType(kind)
                        .build());
                Map<String, Object> body = new LinkedHashMap<>();
                body.put("artifact_type", response.getArtifactType().name());
                body.put("version", response.getVersion());
                body.put("content_type", response.getContentType());
                body.put("content_text", response.getContentText());
                body.put("content_json", structMap(response.getContentJson()));
                body.put("content_hash", response.getContentHash());
                return ResponseEntity.ok().contentType(MediaType.APPLICATION_JSON)
                        .body(new ObjectMapper().writeValueAsString(body));
            });
        } catch (StatusRuntimeException error) {
            if (error.getStatus().getCode() == Status.Code.UNAVAILABLE) {
                return ResponseEntity.status(503).contentType(MediaType.APPLICATION_JSON)
                        .body("{\"error_code\":\"AGENT_UNAVAILABLE\",\"message\":\"artifact service is not configured\"}");
            }
            return status(error.getStatus());
        } catch (Exception error) {
            return ResponseEntity.status(503).contentType(MediaType.APPLICATION_JSON)
                    .body("{\"error_code\":\"AGENT_UNAVAILABLE\",\"message\":\"artifact service is not configured\"}");
        }
    }

    @PostMapping("/api/v1/episodes/{episodeId}/retry-diagnosis")
    public ResponseEntity<String> retryDiagnosis(HttpServletRequest request, @PathVariable String episodeId, @RequestBody(required = false) byte[] body) {
        return call(() -> json(bridge.systemApi().retryDiagnosis(jsonCall(request, episodeId, body))));
    }

    @PostMapping("/api/v1/episodes/{episodeId}/retry-archive")
    public ResponseEntity<String> retryArchive(HttpServletRequest request, @PathVariable String episodeId, @RequestBody(required = false) byte[] body) {
        return call(() -> json(bridge.systemApi().retryArchive(jsonCall(request, episodeId, body))));
    }

    private ResponseEntity<String> cached(String scope, String id, RevisionCache.Loader loader) {
        return call(() -> cache.around(scope, id, loader));
    }

    private ResponseEntity<String> call(java.util.concurrent.Callable<ResponseEntity<String>> action) {
        try {
            return bridge.callGo(action);
        } catch (StatusRuntimeException error) {
            return status(error.getStatus());
        } catch (Exception error) {
            return status(Status.INTERNAL.withDescription(error.getMessage()));
        }
    }

    private static ResponseEntity<String> json(JsonResult result) {
        if (result.getHttpStatus() == 204) {
            return ResponseEntity.noContent().build();
        }
        return ResponseEntity.status(result.getHttpStatus()).contentType(MediaType.APPLICATION_JSON).body(result.getBody());
    }

    private static JsonCall jsonCall(HttpServletRequest request, String id, byte[] body) {
        return jsonCall(request, id, body == null ? "" : new String(body, StandardCharsets.UTF_8));
    }

    private static JsonCall jsonCall(HttpServletRequest request, String id, String body) {
        JsonCall.Builder builder = JsonCall.newBuilder();
        if (id != null) {
            builder.setId(id);
        }
        if (request.getQueryString() != null) {
            builder.setQuery(request.getQueryString());
        }
        if (body != null) {
            builder.setBody(body);
        }
        return builder.build();
    }

    private static Map<String, Object> structMap(Struct struct) {
        Map<String, Object> out = new LinkedHashMap<>();
        struct.getFieldsMap().forEach((key, value) -> out.put(key, structValue(value)));
        return out;
    }

    private static Object structValue(Value value) {
        return switch (value.getKindCase()) {
            case NULL_VALUE -> null;
            case NUMBER_VALUE -> value.getNumberValue();
            case STRING_VALUE -> value.getStringValue();
            case BOOL_VALUE -> value.getBoolValue();
            case STRUCT_VALUE -> structMap(value.getStructValue());
            case LIST_VALUE -> value.getListValue().getValuesList().stream().map(PublicApiController::structValue).toList();
            default -> null;
        };
    }

    private static ResponseEntity<String> status(Status status) {
        int http = switch (status.getCode()) {
            case INVALID_ARGUMENT -> 400;
            case NOT_FOUND -> 404;
            case ALREADY_EXISTS, FAILED_PRECONDITION -> 409;
            case RESOURCE_EXHAUSTED -> 429;
            case DEADLINE_EXCEEDED -> 504;
            case UNAVAILABLE -> 503;
            default -> 502;
        };
        String code = status.getCode().name();
        String message = status.getDescription() == null ? code : status.getDescription();
        return ResponseEntity.status(http).contentType(MediaType.APPLICATION_JSON)
                .body("{\"error_code\":\"" + code + "\",\"message\":\"" + message.replace("\"", "'") + "\"}");
    }
}

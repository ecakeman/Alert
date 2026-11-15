package com.alertflow.integration.cache;

import java.net.URI;
import java.time.Duration;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicBoolean;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.annotation.PreDestroy;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecords;
import org.apache.kafka.clients.consumer.KafkaConsumer;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.stereotype.Component;
import redis.clients.jedis.JedisPooled;

@Component
public class RevisionCache implements AutoCloseable {
    private static final String ADVANCE = """
            local current = redis.call('GET', KEYS[1])
            local next = tonumber(ARGV[1])
            if current and tonumber(current) >= next then
              return 0
            end
            redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
            if ARGV[3] ~= '' then
              redis.call('SET', KEYS[2], ARGV[3], 'PX', ARGV[2])
            end
            return 1
            """;
    private final JedisPooled redis;
    private final ConcurrentHashMap<String, Long> revisions = new ConcurrentHashMap<>();
    private final AtomicBoolean closed = new AtomicBoolean();
    private final ObjectMapper json = new ObjectMapper();
    private final Thread consumer;

    public RevisionCache() {
        String redisURL = env("INTEGRATION_REDIS_URL", "");
        this.redis = redisURL.isBlank() ? null : new JedisPooled(URI.create(redisURL));
        String brokers = env("INTEGRATION_KAFKA_BROKERS", "");
        if (brokers.isBlank()) {
            this.consumer = null;
            return;
        }
        this.consumer = Thread.ofVirtual().name("revision-cache").start(this::consume);
    }

    public ResponseEntity<String> around(String scope, String id, Loader loader) throws Exception {
        String key = scope + ":" + id;
        if (redis != null) {
            try {
                Long known = revisions.get(key);
                String pointer = redis.get(pointer(scope, id));
                long version = pointer == null ? -1 : Long.parseLong(pointer);
                if (known != null && known == version) {
                    String hit = redis.get(value(scope, id, version));
                    if (hit != null) {
                        return ResponseEntity.ok().contentType(MediaType.APPLICATION_JSON).body(hit);
                    }
                }
            } catch (RuntimeException ignored) {
                return loader.load();
            }
        }
        ResponseEntity<String> fresh = loader.load();
        if (redis != null && fresh.getStatusCode().value() == 200 && fresh.getBody() != null) {
            try {
                long version = revisions.getOrDefault(key, 0L);
                long ttl = 5000 + ThreadLocalRandom.current().nextLong(1000);
                redis.eval(ADVANCE, java.util.List.of(pointer(scope, id), value(scope, id, version)),
                        java.util.List.of(Long.toString(version), Long.toString(ttl), fresh.getBody()));
            } catch (RuntimeException ignored) {
                return fresh;
            }
        }
        return fresh;
    }

    private void consume() {
        var props = new java.util.Properties();
        props.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, env("INTEGRATION_KAFKA_BROKERS", "127.0.0.1:9092"));
        props.put(ConsumerConfig.GROUP_ID_CONFIG, "alertflow-revision-cache");
        props.put(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "latest");
        props.put(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class.getName());
        props.put(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, StringDeserializer.class.getName());
        try (KafkaConsumer<String, String> kafka = new KafkaConsumer<>(props)) {
            kafka.subscribe(java.util.List.of(env("INTEGRATION_REVISION_TOPIC", "alertflow.system-resource-revision-changed.v1")));
            while (!closed.get()) {
                ConsumerRecords<String, String> records = kafka.poll(Duration.ofMillis(500));
                records.forEach(record -> apply(record.value()));
            }
        } catch (RuntimeException ignored) {
            // Redis 或 Kafka 不可用时，读取路径直接查询 Go。
        }
    }

    private void apply(String payload) {
        try {
            JsonNode node = json.readTree(payload);
            String scope = node.path("scope").asText();
            String id = node.path("resource_id").asText();
            long revision = node.path("revision").asLong();
            if (scope.isBlank() || id.isBlank() || revision <= 0) {
                return;
            }
            String key = scope + ":" + id;
            revisions.merge(key, revision, Math::max);
            if (redis != null) {
                long ttl = 5000 + ThreadLocalRandom.current().nextLong(1000);
                redis.eval(ADVANCE, java.util.List.of(pointer(scope, id), value(scope, id, revision)),
                        java.util.List.of(Long.toString(revision), Long.toString(ttl), ""));
            }
        } catch (Exception ignored) {
            // 旧事件或坏消息不回退指针。
        }
    }

    @Override
    @PreDestroy
    public void close() {
        closed.set(true);
        if (consumer != null) {
            consumer.interrupt();
        }
        if (redis != null) {
            redis.close();
        }
    }

    private static String pointer(String scope, String id) {
        return "af:cache:revision:" + scope + ":" + id;
    }

    private static String value(String scope, String id, long revision) {
        return "af:api:" + scope + ":" + id + ":v" + revision;
    }

    private static String env(String key, String fallback) {
        String value = System.getenv(key);
        return value == null || value.isBlank() ? fallback : value;
    }

    @FunctionalInterface
    public interface Loader {
        ResponseEntity<String> load() throws Exception;
    }
}

package com.alertflow.integration.cache;

import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.time.Duration;
import java.util.HexFormat;
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
    /** 空 query 的固定 token：null/空/空白 query 共用同一个稳定结果键。 */
    private static final String EMPTY_QUERY = "none";
    /** query hash 取 SHA-256 前 16 个 hex 字符（64 bit），避免把原始长 query 直接拼进 Redis key。 */
    private static final int QUERY_HASH_HEX_LENGTH = 16;
    private final JedisPooled redis;
    /** Kafka 版本事件推进的资源版本（取最大值）。 */
    private final ConcurrentHashMap<String, Long> revisions = new ConcurrentHashMap<>();
    // 指针的本地副本：命中路径据此判断是否可以直接读值键，省掉一次 Redis 往返。
    private final ConcurrentHashMap<String, Long> pointers = new ConcurrentHashMap<>();
    private final AtomicBoolean closed = new AtomicBoolean();
    private final ObjectMapper json = new ObjectMapper();
    private final Thread consumer;

    public RevisionCache() {
        this(connect(env("INTEGRATION_REDIS_URL", "")), true);
    }

    /** 测试用：注入 Redis（可为 null），不启动 Kafka 版本事件消费线程。 */
    RevisionCache(JedisPooled redis) {
        this(redis, false);
    }

    private RevisionCache(JedisPooled redis, boolean consumeRevisionEvents) {
        this.redis = redis;
        String brokers = consumeRevisionEvents ? env("INTEGRATION_KAFKA_BROKERS", "") : "";
        this.consumer = brokers.isBlank() ? null : Thread.ofVirtual().name("revision-cache").start(this::consume);
    }

    /**
     * 缓存身份 = 资源（scope + id，决定 revision pointer）+ 查询（query，决定结果值键）+ 版本（Go revision）。
     * 同一个资源的不同 query 共享 pointer，因此 revision 一推进，该资源下所有 query 变体自然失效。
     */
    public ResponseEntity<String> around(String scope, String id, String query, Loader loader) throws Exception {
        String key = scope + ":" + id;
        String queryHash = queryHash(query);
        // 命中判据是「本地已知该资源的 revision pointer」。指针只由 Kafka 版本事件推进，
        // 或在回源后从 Redis 指针读回；冷启动后第一次回源会补齐指针，之后命中不再访问 Go。
        Long current = cachedPointer(key);
        if (current != null) {
            String hit = redisGet(valueKey(scope, id, queryHash, current));
            if (hit != null) {
                return ResponseEntity.ok().contentType(MediaType.APPLICATION_JSON).body(hit);
            }
        }
        // 简化方案：不做 singleflight，同一 query 的并发未命中各自回源。
        ResponseEntity<String> fresh = loader.load();
        if (fresh.getStatusCode().value() == 200 && fresh.getBody() != null) {
            // 用 Go 的真实版本（指针）写缓存，不能用进程内 map，否则会把指针写回 0。
            long version = refreshPointer(scope, id);
            long ttl = 5000 + ThreadLocalRandom.current().nextLong(1000);
            redisAdvance(pointer(scope, id), valueKey(scope, id, queryHash, version), version, ttl, fresh.getBody());
        }
        return fresh;
    }

    /** 命中路径只用本地已知指针，避免每个请求多一次 Redis 往返。 */
    private Long cachedPointer(String key) {
        Long revision = revisions.get(key);
        if (revision != null) {
            return revision;
        }
        return pointers.get(key);
    }

    /** 回源后读取指针；指针为空时用回源时刻的本地已知版本兜底。 */
    private long refreshPointer(String scope, String id) {
        String raw = redisGet(pointer(scope, id));
        if (raw != null) {
            try {
                long version = Long.parseLong(raw);
                pointers.put(scope + ":" + id, version);
                return version;
            } catch (NumberFormatException ignored) {
                // 指针被外部写成非数字，退回本地已知版本。
            }
        }
        Long known = revisions.get(scope + ":" + id);
        return known == null ? 0L : known;
    }

    /** 读 Redis；未配置或故障时返回 null，读取路径直接回源，缓存故障不影响 API 可用性。 */
    String redisGet(String key) {
        try {
            return rawRedisGet(key);
        } catch (RuntimeException unavailable) {
            return null;
        }
    }

    String rawRedisGet(String key) {
        return redis == null ? null : redis.get(key);
    }

    /** 推进指针并写结果值；body 为空表示只推进指针（版本事件路径）。写失败只影响后续能否命中。 */
    void redisAdvance(String pointerKey, String valueKey, long revision, long ttlMillis, String body) {
        try {
            rawRedisAdvance(pointerKey, valueKey, revision, ttlMillis, body);
        } catch (RuntimeException unavailable) {
            // 缓存写失败不阻塞当前请求，直接返回回源结果。
        }
    }

    void rawRedisAdvance(String pointerKey, String valueKey, long revision, long ttlMillis, String body) {
        if (redis == null) {
            return;
        }
        redis.eval(ADVANCE, java.util.List.of(pointerKey, valueKey),
                java.util.List.of(Long.toString(revision), Long.toString(ttlMillis), body));
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
            onRevision(node.path("scope").asText(), node.path("resource_id").asText(), node.path("revision").asLong());
        } catch (Exception ignored) {
            // 旧事件或坏消息不回退指针。
        }
    }

    /** Kafka 版本事件路径：推进本地版本并写 Redis 指针；该资源下所有 query 变体的旧值不再命中。 */
    void onRevision(String scope, String id, long revision) {
        if (scope.isBlank() || id.isBlank() || revision <= 0) {
            return;
        }
        String key = scope + ":" + id;
        revisions.merge(key, revision, Math::max);
        pointers.merge(key, revision, Math::max);
        long ttl = 5000 + ThreadLocalRandom.current().nextLong(1000);
        redisAdvance(pointer(scope, id), valueKey(scope, id, EMPTY_QUERY, revision), revision, ttl, "");
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

    private static JedisPooled connect(String redisURL) {
        return redisURL.isBlank() ? null : new JedisPooled(URI.create(redisURL));
    }

    private static String pointer(String scope, String id) {
        return "af:cache:revision:" + scope + ":" + id;
    }

    private static String valueKey(String scope, String id, String queryHash, long revision) {
        return "af:api:" + scope + ":" + id + ":q" + queryHash + ":v" + revision;
    }

    /** query 的稳定短哈希；空 query 用固定 token，保证同一请求缓存键可复现且不随机。 */
    static String queryHash(String query) {
        if (query == null || query.isBlank()) {
            return EMPTY_QUERY;
        }
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(query.getBytes(StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(digest).substring(0, QUERY_HASH_HEX_LENGTH);
        } catch (NoSuchAlgorithmException impossible) {
            throw new IllegalStateException("SHA-256 must be available", impossible);
        }
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

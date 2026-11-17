package com.alertflow.integration.cache;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.regex.Pattern;

import org.junit.jupiter.api.Test;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import redis.clients.jedis.JedisPooled;
import redis.clients.jedis.exceptions.JedisConnectionException;

/**
 * RevisionCache 的查询键与版本指针行为。
 * 不依赖真实 Redis：用子类替换 Jedis 读写，验证命中判据、query 区分和 Redis 故障降级。
 */
class RevisionCacheTest {
    private static final String ALL = "00000000-0000-0000-0000-000000000000";
    /** 结果值键形状：af:api:&lt;scope&gt;:&lt;id&gt;:q&lt;query-hash&gt;:v&lt;revision&gt;。 */
    private static final Pattern VALUE_KEY =
            Pattern.compile("af:api:services:" + ALL + ":q([0-9a-f]{16}|none):v\\d+");

    @Test
    void sameQueryHitsCacheOnSecondRequest() throws Exception {
        InMemoryRevisionCache cache = new InMemoryRevisionCache();
        cache.onRevision("services", ALL, 1);
        AtomicInteger calls = new AtomicInteger();

        ResponseEntity<String> first = cache.around("services", ALL, "status=ACTIVE", loader(calls, "first"));
        ResponseEntity<String> second = cache.around("services", ALL, "status=ACTIVE", loader(calls, "second"));

        assertEquals(1, calls.get(), "第二次相同 path + query 必须命中缓存，不再调用 loader");
        assertEquals(200, first.getStatusCode().value());
        assertEquals("first", second.getBody(), "命中时返回缓存值，而不是第二次 loader 的结果");
    }

    @Test
    void differentQueriesUseSeparateCacheEntries() throws Exception {
        InMemoryRevisionCache cache = new InMemoryRevisionCache();
        cache.onRevision("services", ALL, 1);
        AtomicInteger calls = new AtomicInteger();

        assertEquals("active", call(cache, "status=ACTIVE&page=1", calls, "active"));
        assertEquals("archived", call(cache, "status=ARCHIVED&page=1", calls, "archived"));
        assertEquals(2, calls.get(), "相同 path、不同 query 必须各自回源");
        assertEquals("active", call(cache, "status=ACTIVE&page=1", calls, "unexpected"), "相同 query 第二次应命中");

        Set<String> valueKeys = cache.keys().stream().filter(key -> key.startsWith("af:api:")).collect(java.util.stream.Collectors.toSet());
        assertEquals(2, valueKeys.size(), valueKeys.toString());
        assertTrue(valueKeys.stream().allMatch(key -> VALUE_KEY.matcher(key).matches()), valueKeys.toString());
        assertTrue(valueKeys.stream().noneMatch(key -> key.contains("status=ACTIVE") || key.contains("status=ARCHIVED")),
                "原始 query 不能直接出现在 Redis 键里：" + valueKeys);
    }

    @Test
    void revisionBumpInvalidatesAllQueryVariants() throws Exception {
        InMemoryRevisionCache cache = new InMemoryRevisionCache();
        cache.onRevision("services", ALL, 1);
        AtomicInteger calls = new AtomicInteger();

        call(cache, "status=ACTIVE", calls, "active-v1");
        call(cache, "status=ARCHIVED", calls, "archived-v1");
        assertEquals(2, calls.get());

        // Go 推进该资源 revision 后，同一资源下不同 query 的旧版本值都不得继续命中。
        cache.onRevision("services", ALL, 2);

        assertEquals("active-v2", call(cache, "status=ACTIVE", calls, "active-v2"));
        assertEquals("archived-v2", call(cache, "status=ARCHIVED", calls, "archived-v2"));
        assertEquals(4, calls.get(), "revision 增加后该资源下所有 query 变体都必须重新回源");

        assertEquals("active-v2", call(cache, "status=ACTIVE", calls, "unexpected"));
        assertEquals("archived-v2", call(cache, "status=ARCHIVED", calls, "unexpected"));
        assertEquals(4, calls.get(), "新 revision 下的结果应再次命中");
    }

    @Test
    void emptyQueryUsesStableDefaultKey() throws Exception {
        InMemoryRevisionCache cache = new InMemoryRevisionCache();
        cache.onRevision("services", ALL, 1);
        AtomicInteger calls = new AtomicInteger();

        assertEquals("no-query", call(cache, null, calls, "no-query"));
        assertEquals("no-query", call(cache, "", calls, "unexpected"), "空 query 必须命中 null query 的缓存项");
        assertEquals("no-query", call(cache, null, calls, "unexpected"));

        assertEquals(1, calls.get(), "null/空 query 必须共用一个稳定的默认缓存键，不能生成随机键");
        assertTrue(cache.keys().stream().anyMatch(key -> key.contains(":qnone:")), cache.keys().toString());
    }

    @Test
    void redisFailureFallsBackToLoader() throws Exception {
        RevisionCache cache = new BrokenRevisionCache();
        cache.onRevision("services", ALL, 1);
        AtomicInteger calls = new AtomicInteger();

        ResponseEntity<String> first = cache.around("services", ALL, "status=ACTIVE", loader(calls, "fresh-1"));
        ResponseEntity<String> second = cache.around("services", ALL, "status=ACTIVE", loader(calls, "fresh-2"));

        assertEquals(2, calls.get(), "Redis 读写都抛异常时必须回源，而不是让 API 失败");
        assertEquals(200, first.getStatusCode().value());
        assertEquals("fresh-2", second.getBody());
    }

    @Test
    void queryHashIsStableShortAndDoesNotEmbedQuery() {
        String hash = RevisionCache.queryHash("status=ACTIVE&page=1&limit=50");

        assertEquals(16, hash.length());
        assertTrue(hash.matches("[0-9a-f]{16}"), hash);
        assertEquals(hash, RevisionCache.queryHash("status=ACTIVE&page=1&limit=50"), "同一 query 的哈希必须稳定");
        assertNotEquals(RevisionCache.queryHash("status=ACTIVE"), RevisionCache.queryHash("status=ARCHIVED"));
        assertEquals("none", RevisionCache.queryHash(null));
        assertEquals("none", RevisionCache.queryHash(""));
        assertFalse(hash.contains("status"));
    }

    private static RevisionCache.Loader loader(AtomicInteger calls, String body) {
        // 计数放在 loader 内：loader 被调用几次就代表真正回源 Go 几次。
        return () -> {
            calls.incrementAndGet();
            return ResponseEntity.ok().contentType(MediaType.APPLICATION_JSON).body(body);
        };
    }

    private static String call(RevisionCache cache, String query, AtomicInteger calls, String body) throws Exception {
        return cache.around("services", ALL, query, loader(calls, body)).getBody();
    }

    /** 内存版缓存：行为对齐 Redis 的指针键与结果值键（指针不回退、空 body 只推进指针）。 */
    private static final class InMemoryRevisionCache extends RevisionCache {
        private final Map<String, String> store = new ConcurrentHashMap<>();

        InMemoryRevisionCache() {
            super((JedisPooled) null);
        }

        @Override
        String rawRedisGet(String key) {
            return store.get(key);
        }

        @Override
        void rawRedisAdvance(String pointerKey, String valueKey, long revision, long ttlMillis, String body) {
            long current = store.containsKey(pointerKey) ? Long.parseLong(store.get(pointerKey)) : -1L;
            if (revision > current) {
                store.put(pointerKey, Long.toString(revision));
            }
            if (body != null && !body.isEmpty()) {
                store.put(valueKey, body);
            }
        }

        Set<String> keys() {
            return store.keySet();
        }
    }

    /** 模拟 Redis 完全不可用：读写都抛连接异常。 */
    private static final class BrokenRevisionCache extends RevisionCache {
        BrokenRevisionCache() {
            super((JedisPooled) null);
        }

        @Override
        String rawRedisGet(String key) {
            throw new JedisConnectionException("redis down");
        }

        @Override
        void rawRedisAdvance(String pointerKey, String valueKey, long revision, long ttlMillis, String body) {
            throw new JedisConnectionException("redis down");
        }
    }
}

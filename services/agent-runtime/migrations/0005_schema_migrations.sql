-- 0005: 迁移自举（schema_migrations）
-- runner 在回放任何文件前先自举本表（与 0001 的 schema 创建同批前置），
-- 记录 version + name + SHA-256 checksum；重复回放 no-op（design §6.1，CR-REG-49）。
-- 纯 SQL 裸迁移：不含 BEGIN/COMMIT；禁止 DROP；无 CASE WHEN；整体执行（CR-REG-37）。

CREATE TABLE IF NOT EXISTS cognitive.schema_migrations (
    version    integer     NOT NULL,
    name       text        NOT NULL,
    checksum   text        NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT schema_migrations_pk PRIMARY KEY (version)
);
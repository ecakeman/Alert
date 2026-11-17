#!/bin/sh
set -eu
export PGPASSWORD="${POSTGRES_PASSWORD}"
# 即使 healthcheck 已通过，postgres 在 initdb 收尾重启的瞬间仍可能拒绝连接，
# 这里做有界重试，避免把启动竞态误报成 E2E 失败。
i=0
until psql -h postgres -U "${POSTGRES_USER}" -d postgres -c 'select 1' >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge 30 ]; then
    echo "postgres not reachable after 30 attempts" >&2
    exit 1
  fi
  sleep 1
done
psql -h postgres -U "${POSTGRES_USER}" -d postgres -v ON_ERROR_STOP=1 \
  -v gateway_password="${GATEWAY_PASSWORD}" -v knowledge_password="${KNOWLEDGE_PASSWORD}" <<'SQL'
SELECT format('CREATE ROLE gateway_app LOGIN PASSWORD %L', :'gateway_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'gateway_app')\gexec
SELECT format('CREATE ROLE knowledge_app LOGIN PASSWORD %L', :'knowledge_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'knowledge_app')\gexec
SELECT 'CREATE DATABASE knowledge OWNER knowledge_app'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'knowledge')\gexec
GRANT CONNECT ON DATABASE alertflow TO gateway_app;
GRANT CREATE ON DATABASE alertflow TO gateway_app;
SQL
psql -h postgres -U "${POSTGRES_USER}" -d knowledge -v ON_ERROR_STOP=1 <<'SQL'
CREATE EXTENSION IF NOT EXISTS vector;
GRANT ALL ON SCHEMA public TO knowledge_app;
SQL

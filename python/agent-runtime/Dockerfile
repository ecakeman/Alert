FROM python:3.12-slim AS builder

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /build
COPY pyproject.toml README.md ./
COPY wheels/ /wheels

# 只安装运行依赖（不构建本项目，避免构建隔离再次联网拉 setuptools）。
RUN python -c "import tomllib;d=tomllib.load(open('pyproject.toml','rb'));open('/tmp/req.txt','w').write('\n'.join(d['project']['dependencies']))"

ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
RUN if [ -d /wheels ] && [ -n "$(ls -A /wheels)" ]; then \
      pip install --no-cache-dir --no-index --find-links=/wheels setuptools wheel && \
      pip install --no-cache-dir --no-index --find-links=/wheels --no-build-isolation -r /tmp/req.txt ; \
    else \
      pip install --no-cache-dir --index-url "$PIP_INDEX_URL" -r /tmp/req.txt ; \
    fi

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/src

WORKDIR /app
COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY src ./src
COPY migrations ./migrations

EXPOSE 8080 50052
CMD ["python", "-m", "alert_agent.bootstrap.app"]
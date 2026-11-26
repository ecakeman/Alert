FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /app
RUN pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple uv

COPY pyproject.toml README.md ./
COPY src ./src
RUN uv pip install --system --no-cache --index-url https://pypi.tuna.tsinghua.edu.cn/simple -e .

EXPOSE 8080 50052
CMD ["python", "-m", "alert_agent.bootstrap.app"]

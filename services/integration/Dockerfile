# Java Integration：JDK + Maven 构建阶段产出可执行 jar，JRE 运行阶段仅保留运行时依赖。
# 构建上下文是仓库根目录（Compose 中 context: ..），因此 proto/ 与 integration/ 都按仓库根路径 COPY。
# 基础镜像可用 --build-arg JDK_IMAGE=... / JRE_IMAGE=... 覆盖（Compose 见 compose.yaml build.args）。
ARG JDK_IMAGE=eclipse-temurin:25-jdk
ARG JRE_IMAGE=eclipse-temurin:25-jre
FROM ${JDK_IMAGE} AS build
WORKDIR /src
RUN apt-get update \
    && apt-get install -y --no-install-recommends maven \
    && rm -rf /var/lib/apt/lists/*
COPY contracts/proto /contracts/proto
COPY services/integration /src/integration
WORKDIR /src/integration
# maven-settings.xml 配置阿里云镜像；-B 批处理输出，跳过测试只验证可构建。
RUN mvn -B -s maven-settings.xml -DskipTests package

FROM ${JRE_IMAGE}
# curl 供 HEALTHCHECK 访问 Actuator liveness；运行时使用非 root 用户（uid 10002）。
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10002 integration \
    && useradd --system --uid 10002 --gid integration --create-home --home-dir /home/integration --shell /usr/sbin/nologin integration
WORKDIR /app
COPY --from=build /src/integration/target/integration-0.1.0.jar /app/integration.jar
USER 10002:10002
EXPOSE 8081 9091
HEALTHCHECK --interval=10s --timeout=3s --start-period=45s --retries=12 \
    CMD curl -fsS http://127.0.0.1:8081/actuator/health/liveness || exit 1
ENTRYPOINT ["java", "-jar", "/app/integration.jar"]

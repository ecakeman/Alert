#!/usr/bin/env bash
# 一次性创建按域 topic；脚本幂等，可重复执行。
set -euo pipefail

BOOTSTRAP="${KAFKA_BOOTSTRAP:-kafka:29092}"
TOPICS="${KAFKA_TOPICS:-alertflow.registry.events.v1 alertflow.monitor.events.v1 alertflow.episode.events.v1 alertflow.task.events.v1}"

for topic in $TOPICS; do
  /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "$BOOTSTRAP" \
    --create --if-not-exists \
    --topic "$topic" \
    --partitions 3 \
    --replication-factor 1
done

/opt/kafka/bin/kafka-topics.sh --bootstrap-server "$BOOTSTRAP" --list
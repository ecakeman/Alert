// Package messaging 提供与业务无关的 Kafka 收发原语。
//
// 约束：
//   - 每个域只认一个发布 topic（SenderConfig.Topic）；
//   - 消费侧支持多 topic 订阅与独立 consumer group（ConsumerConfig.Topics/GroupID）；
//   - 消息体用原始 []byte，编解码由服务层通过 contracts 注入；
//   - 消费失败不提交 offset，不阻塞后续消息（业务幂等/重试负责补偿）。
package messaging

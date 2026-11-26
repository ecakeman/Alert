"""A2A 协议层（T22）：agent 卡片、message/send 服务端映射及知识调用
客户端（CR-GW-08/08a，CR-KNOW-01~09）。"""

from alert_agent.infrastructure.a2a.card import AgentCard, AgentSkill, build_agent_card

__all__ = ["AgentCard", "AgentSkill", "build_agent_card"]
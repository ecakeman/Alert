"""A2A protocol surface (T22): agent card, message/send server mapping and
the knowledge-invocation client (CR-GW-08/08a, CR-KNOW-01~09)."""

from alert_agent.infrastructure.a2a.card import AgentCard, AgentSkill, build_agent_card

__all__ = ["AgentCard", "AgentSkill", "build_agent_card"]
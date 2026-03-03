import { create } from 'zustand';
import type { AgentState, AgentStatus, DashboardEvent } from '@/lib/types';

const MAX_EVENTS_PER_AGENT = 500;
const MAX_RECENT_EVENTS = 100;

interface AgentStore {
  agents: Map<string, AgentState>;
  agentEvents: Map<string, DashboardEvent[]>;
  agentConnectionInfo: Map<string, Record<string, string>>;
  recentEvents: DashboardEvent[];
  setAgents: (agents: AgentState[]) => void;
  handleEvent: (event: DashboardEvent) => void;
}

export const useAgentStore = create<AgentStore>((set) => ({
  agents: new Map(),
  agentEvents: new Map(),
  agentConnectionInfo: new Map(),
  recentEvents: [],

  setAgents: (agents: AgentState[]) => {
    const map = new Map<string, AgentState>();
    for (const a of agents) {
      map.set(a.agent_id, a);
    }
    set({ agents: map });
  },

  handleEvent: (event: DashboardEvent) => {
    set((state) => {
      const agents = new Map(state.agents);
      const agentEvents = new Map(state.agentEvents);
      const agentConnectionInfo = new Map(state.agentConnectionInfo);

      agents.set(event.agent_id, {
        agent_id: event.agent_id,
        agent_type: event.agent_type,
        agent_name: event.agent_name,
        status: event.status as AgentStatus,
        last_event: event.timestamp,
      });

      // Extract connection info from session.start events
      if (event.type === 'session.start' && event.data) {
        agentConnectionInfo.set(event.agent_id, event.data as Record<string, string>);
      }

      if (event.type !== 'output.stream') {
        const existing = agentEvents.get(event.agent_id) ?? [];
        const updated = [...existing, event].slice(-MAX_EVENTS_PER_AGENT);
        agentEvents.set(event.agent_id, updated);
      }

      const recentEvents = [event, ...state.recentEvents].slice(0, MAX_RECENT_EVENTS);

      // Browser notification for questions
      if (event.type === 'question.asked' && typeof window !== 'undefined' && Notification.permission === 'granted') {
        const data = event.data as Record<string, unknown> | undefined;
        new Notification(`${event.agent_name || 'Agent'} needs input`, {
          body: (data?.question as string) ?? 'Response needed',
        });
      }

      return { agents, agentEvents, agentConnectionInfo, recentEvents };
    });
  },
}));

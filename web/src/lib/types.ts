export type AgentStatus = "queued" | "active" | "waiting" | "done" | "error";

export interface DashboardEvent {
  id: string;
  type: string;
  timestamp: string;
  agent_id: string;
  agent_type: string;
  agent_name: string;
  data?: Record<string, unknown>;
  status: AgentStatus;
}

export interface AgentState {
  agent_id: string;
  agent_type: string;
  agent_name: string;
  status: AgentStatus;
  last_event: string;
}

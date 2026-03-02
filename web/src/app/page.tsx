'use client';

import { useAgentStore } from '@/store/agents';
import { AgentCard } from '@/components/agent-card';
import type { AgentState, AgentStatus } from '@/lib/types';

const COLUMNS: { status: AgentStatus; label: string; emptyText: string }[] = [
  { status: 'queued', label: 'Queued', emptyText: 'No queued agents' },
  { status: 'active', label: 'Active', emptyText: 'No active agents' },
  { status: 'waiting', label: 'Waiting', emptyText: 'No agents waiting' },
  { status: 'done', label: 'Done', emptyText: 'No completed agents' },
];

export default function KanbanPage() {
  const agents = useAgentStore((s) => s.agents);
  const allAgents = Array.from(agents.values());

  function agentsForStatus(status: AgentStatus): AgentState[] {
    return allAgents.filter((a) => a.status === status);
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Agents</h1>

      {allAgents.length === 0 && (
        <div className="text-gray-500 text-center py-12">
          No agents connected. Start one with: <code className="font-mono text-gray-400">awayteam agent --name &quot;my-task&quot; claude</code>
        </div>
      )}

      {allAgents.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {COLUMNS.map((col) => {
            const colAgents = agentsForStatus(col.status);
            return (
              <div key={col.status}>
                <h2 className="text-sm font-semibold uppercase tracking-wide text-gray-400 mb-3">
                  {col.label}
                  {colAgents.length > 0 && (
                    <span className="ml-2 text-gray-500">({colAgents.length})</span>
                  )}
                </h2>
                <div className="space-y-3">
                  {colAgents.length === 0 && (
                    <p className="text-xs text-gray-600 py-4 text-center">{col.emptyText}</p>
                  )}
                  {colAgents.map((agent) => (
                    <AgentCard key={agent.agent_id} agent={agent} />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

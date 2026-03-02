'use client';

import type { AgentState } from '@/lib/types';
import { useEffect, useState } from 'react';

function timeAgo(timestamp: string): string {
  const diff = Date.now() - new Date(timestamp).getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

function statusBorderColor(status: string): string {
  switch (status) {
    case 'active': return 'border-l-emerald-500';
    case 'waiting': return 'border-l-amber-500';
    case 'error': return 'border-l-red-500';
    case 'done': return 'border-l-gray-500';
    default: return 'border-l-blue-500';
  }
}

function statusDotColor(status: string): string {
  switch (status) {
    case 'active': return 'bg-emerald-500';
    case 'waiting': return 'bg-amber-500 animate-pulse';
    case 'error': return 'bg-red-500';
    case 'done': return 'bg-gray-500';
    default: return 'bg-blue-500';
  }
}

interface AgentCardProps {
  agent: AgentState;
}

export function AgentCard({ agent }: AgentCardProps) {
  const [ago, setAgo] = useState(timeAgo(agent.last_event));

  useEffect(() => {
    const interval = setInterval(() => {
      setAgo(timeAgo(agent.last_event));
    }, 5000);
    return () => clearInterval(interval);
  }, [agent.last_event]);

  return (
    <a href={`/agents/${encodeURIComponent(agent.agent_id)}`}>
      <div
        className={`rounded-lg bg-gray-800 border-l-4 ${statusBorderColor(agent.status)} p-4 hover:bg-gray-750 transition-colors cursor-pointer`}
      >
        <div className="flex items-start justify-between mb-2">
          <div className="flex items-center gap-2">
            <span className={`inline-block h-2.5 w-2.5 rounded-full ${statusDotColor(agent.status)}`} />
            <h3 className="text-lg font-bold text-gray-100 truncate">
              {agent.agent_name || agent.agent_id.slice(0, 8)}
            </h3>
          </div>
          <span className="text-xs text-gray-500 whitespace-nowrap ml-2">
            {ago}
          </span>
        </div>

        <div className="flex items-center gap-2 text-sm">
          <span className="rounded-full bg-gray-700 px-2 py-0.5 text-xs text-gray-400">
            {agent.agent_type || 'generic'}
          </span>
          <span className="text-gray-500 capitalize">{agent.status}</span>
        </div>
      </div>
    </a>
  );
}

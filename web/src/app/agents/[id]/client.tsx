'use client';

import { use } from 'react';
import { useAgentStore } from '@/store/agents';
import { ConversationView } from '@/components/conversation-view';
import { ResponseInput } from '@/components/response-input';
import Link from 'next/link';

export default function AgentPageClient({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const agentId = decodeURIComponent(id);

  const agent = useAgentStore((s) => s.agents.get(agentId));
  const events = useAgentStore((s) => s.agentEvents.get(agentId) ?? []);

  return (
    <div className="flex flex-col h-[calc(100vh-73px)]">
      <div className="border-b border-gray-800 px-6 py-4 flex items-center gap-4">
        <Link href="/" className="text-gray-400 hover:text-white text-sm">
          &larr; Back
        </Link>
        <h1 className="text-xl font-bold">
          {agent?.agent_name || agentId.slice(0, 8)}
        </h1>
        {agent && (
          <>
            <span className="rounded-full bg-gray-700 px-2 py-0.5 text-xs text-gray-400">
              {agent.agent_type}
            </span>
            <span className="text-sm text-gray-500 capitalize">{agent.status}</span>
          </>
        )}
      </div>

      <div className="flex-1 overflow-y-auto px-6 py-4">
        <ConversationView events={events} />
      </div>

      <ResponseInput agentId={agentId} isWaiting={agent?.status === 'waiting'} />
    </div>
  );
}

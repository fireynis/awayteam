'use client';

import { useSearchParams } from 'next/navigation';
import { Suspense, useState } from 'react';
import { useAgentStore } from '@/store/agents';
import { ConversationView } from '@/components/conversation-view';
import { ResponseInput } from '@/components/response-input';
import { TerminalView } from '@/components/terminal-view';
import { ConnectionInfo } from '@/components/connection-info';
import Link from 'next/link';

function AgentPageContent() {
  const searchParams = useSearchParams();
  const agentId = searchParams.get('id') ?? '';
  const [activeTab, setActiveTab] = useState<'terminal' | 'chat'>('terminal');

  const agent = useAgentStore((s) => s.agents.get(agentId));
  const events = useAgentStore((s) => s.agentEvents.get(agentId) ?? []);
  const connectionInfo = useAgentStore((s) => s.agentConnectionInfo.get(agentId));

  if (!agentId) {
    return (
      <div className="text-gray-500 text-center py-12">
        No agent ID specified. <Link href="/" className="text-blue-400 hover:underline">Go back</Link>
      </div>
    );
  }

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

        {/* Tab bar */}
        <div className="ml-auto flex rounded-lg bg-gray-800 p-0.5">
          <button
            onClick={() => setActiveTab('terminal')}
            className={`px-3 py-1 text-sm rounded-md transition-colors ${
              activeTab === 'terminal'
                ? 'bg-gray-700 text-white'
                : 'text-gray-400 hover:text-gray-200'
            }`}
          >
            Terminal
          </button>
          <button
            onClick={() => setActiveTab('chat')}
            className={`px-3 py-1 text-sm rounded-md transition-colors ${
              activeTab === 'chat'
                ? 'bg-gray-700 text-white'
                : 'text-gray-400 hover:text-gray-200'
            }`}
          >
            Chat
          </button>
        </div>
      </div>

      {connectionInfo && (
        <ConnectionInfo
          hostname={connectionInfo.hostname}
          username={connectionInfo.username}
          tmuxSession={connectionInfo.tmux_session}
          sshCommand={connectionInfo.ssh_command}
          tmuxCommand={connectionInfo.tmux_command}
        />
      )}

      {activeTab === 'terminal' ? (
        <div className="flex-1 min-h-0">
          <TerminalView
            agentId={agentId}
            tmuxSession={connectionInfo?.tmux_session}
          />
        </div>
      ) : (
        <>
          <div className="flex-1 overflow-y-auto px-6 py-4">
            <ConversationView events={events} />
          </div>
          <ResponseInput agentId={agentId} isWaiting={agent?.status === 'waiting'} />
        </>
      )}
    </div>
  );
}

export default function AgentPage() {
  return (
    <Suspense fallback={<div className="text-gray-500 text-center py-12">Loading...</div>}>
      <AgentPageContent />
    </Suspense>
  );
}

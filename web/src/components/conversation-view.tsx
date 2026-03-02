'use client';

import type { DashboardEvent } from '@/lib/types';

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString();
}

function EventBubble({ event }: { event: DashboardEvent }) {
  const isUser = event.type === 'message.user';
  const isTool = event.type === 'tool.call' || event.type === 'tool.result';
  const isQuestion = event.type === 'question.asked';

  if (isTool) {
    const data = event.data as Record<string, unknown> | undefined;
    return (
      <div className="ml-4 border-l-2 border-gray-700 pl-3 py-1">
        <div className="flex items-center gap-2 text-xs text-gray-500">
          <span className="font-mono">{(data?.tool as string) ?? event.type}</span>
          <span>{formatTime(event.timestamp)}</span>
        </div>
        {data?.result != null && (
          <pre className="mt-1 text-xs text-gray-400 font-mono overflow-x-auto max-h-32 overflow-y-auto">
            {typeof data.result === 'string' ? data.result : JSON.stringify(data.result, null, 2)}
          </pre>
        )}
      </div>
    );
  }

  if (isQuestion) {
    const data = event.data as Record<string, unknown> | undefined;
    return (
      <div className="bg-amber-900/30 border border-amber-700 rounded-lg p-4 my-2">
        <div className="flex items-center gap-2 mb-2">
          <span className="text-amber-400 font-semibold text-sm">Question</span>
          <span className="text-xs text-gray-500">{formatTime(event.timestamp)}</span>
        </div>
        <p className="text-gray-200">{(data?.question as string) ?? 'Awaiting response...'}</p>
        {Array.isArray(data?.options) && (
          <div className="flex flex-wrap gap-2 mt-2">
            {(data.options as string[]).map((opt, i) => (
              <span key={i} className="rounded-full bg-amber-800/50 px-3 py-1 text-sm text-amber-200">
                {opt}
              </span>
            ))}
          </div>
        )}
      </div>
    );
  }

  const bubbleClass = isUser
    ? 'bg-blue-900/50 border-blue-700'
    : 'bg-gray-800 border-gray-700';

  const data = event.data as Record<string, unknown> | undefined;
  const content = (data?.content as string) ?? JSON.stringify(data ?? {});

  return (
    <div className={`rounded-lg border ${bubbleClass} p-3 my-2`}>
      <div className="flex items-center gap-2 mb-1">
        <span className="text-xs font-semibold text-gray-400">
          {isUser ? 'You' : 'Agent'}
        </span>
        <span className="text-xs text-gray-500">{formatTime(event.timestamp)}</span>
      </div>
      <div className="text-sm text-gray-200 whitespace-pre-wrap">{content}</div>
    </div>
  );
}

interface ConversationViewProps {
  events: DashboardEvent[];
}

export function ConversationView({ events }: ConversationViewProps) {
  const conversationEvents = events.filter(
    (e) =>
      e.type === 'message.user' ||
      e.type === 'message.assistant' ||
      e.type === 'tool.call' ||
      e.type === 'tool.result' ||
      e.type === 'question.asked' ||
      e.type === 'question.answered'
  );

  if (conversationEvents.length === 0) {
    return (
      <div className="text-gray-500 text-center py-8">
        No conversation events yet.
      </div>
    );
  }

  return (
    <div className="space-y-1">
      {conversationEvents.map((event) => (
        <EventBubble key={event.id} event={event} />
      ))}
    </div>
  );
}

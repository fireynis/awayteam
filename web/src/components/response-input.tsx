'use client';

import { useState, useRef, useEffect } from 'react';

interface ResponseInputProps {
  agentId: string;
  isWaiting: boolean;
}

export function ResponseInput({ agentId, isWaiting }: ResponseInputProps) {
  const [text, setText] = useState('');
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const wsUrl =
      `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws/agents/${agentId}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onerror = () => ws.close();

    return () => { ws.close(); };
  }, [agentId]);

  function send() {
    if (!text.trim() || !wsRef.current) return;

    wsRef.current.send(JSON.stringify({
      type: 'response',
      text: text + '\n',
    }));
    setText('');
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  return (
    <div className="border-t border-gray-700 p-4 bg-gray-800/50">
      <div className="flex items-center gap-2 mb-2">
        <span className={`h-2 w-2 rounded-full ${connected ? 'bg-emerald-500' : 'bg-red-500'}`} />
        <span className="text-xs text-gray-500">
          {connected ? 'Connected' : 'Disconnected'}
        </span>
        {isWaiting && (
          <span className="text-xs text-amber-400 animate-pulse">Waiting for response...</span>
        )}
      </div>
      <div className="flex gap-2">
        <input
          type="text"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Type a response..."
          className="flex-1 rounded-lg bg-gray-900 border border-gray-700 px-3 py-2 text-sm text-gray-100 placeholder-gray-500 focus:border-blue-500 focus:outline-none"
        />
        <button
          onClick={send}
          disabled={!text.trim() || !connected}
          className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Send
        </button>
      </div>
    </div>
  );
}

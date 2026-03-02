'use client';

import { useEffect, useRef } from 'react';
import { useAgentStore } from '@/store/agents';
import type { AgentState, DashboardEvent } from '@/lib/types';

export function useWebSocket(url: string) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backoffRef = useRef(1000);
  const unmountedRef = useRef(false);

  useEffect(() => {
    if (!url) return;
    unmountedRef.current = false;

    function deriveRestUrl(wsUrl: string): string {
      const parsed = new URL(wsUrl);
      parsed.protocol = parsed.protocol === 'wss:' ? 'https:' : 'http:';
      parsed.pathname = '/api/v1/agents';
      return parsed.toString();
    }

    function connect() {
      if (unmountedRef.current) return;

      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = async () => {
        backoffRef.current = 1000;
        try {
          const res = await fetch(deriveRestUrl(url));
          if (res.ok) {
            const agents: AgentState[] = await res.json();
            useAgentStore.getState().setAgents(agents);
          }
        } catch { /* ignore */ }
      };

      ws.onmessage = (msg) => {
        try {
          const event: DashboardEvent = JSON.parse(msg.data);
          useAgentStore.getState().handleEvent(event);
        } catch { /* ignore */ }
      };

      ws.onclose = () => {
        if (unmountedRef.current) return;
        scheduleReconnect();
      };

      ws.onerror = () => { ws.close(); };
    }

    function scheduleReconnect() {
      if (unmountedRef.current) return;
      const delay = backoffRef.current;
      backoffRef.current = Math.min(delay * 2, 30000);
      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null;
        connect();
      }, delay);
    }

    connect();

    return () => {
      unmountedRef.current = true;
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      if (wsRef.current) wsRef.current.close();
    };
  }, [url]);
}

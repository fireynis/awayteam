'use client';

import { useEffect, useRef, useState } from 'react';
import type { Terminal } from '@xterm/xterm';
import type { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

interface TerminalViewProps {
  agentId: string;
  tmuxSession?: string | null;
}

export function TerminalView({ agentId, tmuxSession }: TerminalViewProps) {
  const termRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected'>('connecting');

  useEffect(() => {
    if (!termRef.current) return;

    let cancelled = false;
    let terminal: Terminal;
    let ws: WebSocket;
    let resizeObserver: ResizeObserver;

    async function init() {
      const [
        { Terminal },
        { FitAddon },
        { WebLinksAddon },
      ] = await Promise.all([
        import('@xterm/xterm'),
        import('@xterm/addon-fit'),
        import('@xterm/addon-web-links'),
      ]);
      if (cancelled || !termRef.current) return;

      terminal = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
        scrollback: 10000,
        theme: {
          background: '#0d1117',
          foreground: '#c9d1d9',
          cursor: '#58a6ff',
          selectionBackground: '#264f78',
        },
        allowProposedApi: true,
      });

      const fitAddon = new FitAddon();
      const webLinksAddon = new WebLinksAddon();
      terminal.loadAddon(fitAddon);
      terminal.loadAddon(webLinksAddon);
      terminal.open(termRef.current);
      fitAddon.fit();

      terminalRef.current = terminal;
      fitAddonRef.current = fitAddon;

      // WebSocket connection
      const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${wsProtocol}//${window.location.host}/api/v1/ws/terminal/${agentId}`;

      ws = new WebSocket(wsUrl);
      ws.binaryType = 'arraybuffer';
      wsRef.current = ws;

      ws.onopen = () => {
        setStatus('connected');
        const dims = fitAddon.proposeDimensions();
        if (dims) {
          ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
        }
      };

      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          terminal.write(new Uint8Array(event.data));
        }
      };

      ws.onclose = () => setStatus('disconnected');
      ws.onerror = () => ws.close();

      terminal.onData((data) => {
        if (ws.readyState === WebSocket.OPEN) {
          const encoder = new TextEncoder();
          ws.send(encoder.encode(data));
        }
      });

      resizeObserver = new ResizeObserver(() => {
        fitAddon.fit();
        const dims = fitAddon.proposeDimensions();
        if (dims && ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
        }
      });
      resizeObserver.observe(termRef.current!);
    }

    init();

    return () => {
      cancelled = true;
      resizeObserver?.disconnect();
      ws?.close();
      terminal?.dispose();
    };
  }, [agentId]);

  const statusColor = {
    connecting: 'text-yellow-400',
    connected: 'text-emerald-400',
    disconnected: 'text-red-400',
  }[status];

  const statusDot = {
    connecting: 'bg-yellow-500',
    connected: 'bg-emerald-500',
    disconnected: 'bg-red-500',
  }[status];

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-4 py-2 bg-gray-900/50 border-b border-gray-800 text-xs">
        <span className={`h-2 w-2 rounded-full ${statusDot}`} />
        <span className={statusColor}>
          {status === 'connected' && tmuxSession
            ? `tmux: ${tmuxSession}`
            : status === 'connected'
              ? 'PTY stream'
              : status}
        </span>
      </div>
      <div ref={termRef} className="flex-1 bg-[#0d1117]" />
    </div>
  );
}

'use client';

import { useState } from 'react';

interface ConnectionInfoProps {
  hostname?: string;
  username?: string;
  tmuxSession?: string;
  sshCommand?: string;
  tmuxCommand?: string;
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <button
      onClick={handleCopy}
      className="text-xs text-gray-400 hover:text-white px-2 py-0.5 rounded bg-gray-700 hover:bg-gray-600"
    >
      {copied ? 'Copied' : 'Copy'}
    </button>
  );
}

export function ConnectionInfo({ hostname, username, tmuxSession, sshCommand, tmuxCommand }: ConnectionInfoProps) {
  const [expanded, setExpanded] = useState(false);

  if (!hostname && !tmuxSession) return null;

  return (
    <div className="border-b border-gray-800 bg-gray-900/30">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-4 py-2 text-xs text-gray-400 hover:text-gray-200"
      >
        <span className="font-mono">{expanded ? '[-]' : '[+]'}</span>
        <span>Connection Info</span>
        {tmuxSession && (
          <span className="text-emerald-500 font-mono">{tmuxSession}</span>
        )}
        {hostname && (
          <span className="text-gray-500">@{hostname}</span>
        )}
      </button>

      {expanded && (
        <div className="px-4 pb-3 space-y-2">
          {tmuxSession && (
            <div className="flex items-center gap-2">
              <span className="text-xs text-gray-500 w-20">tmux:</span>
              <code className="text-xs text-gray-300 font-mono flex-1">{tmuxCommand ?? `tmux attach -t ${tmuxSession}`}</code>
              <CopyButton text={tmuxCommand ?? `tmux attach -t ${tmuxSession}`} />
            </div>
          )}
          {sshCommand && (
            <div className="flex items-center gap-2">
              <span className="text-xs text-gray-500 w-20">SSH:</span>
              <code className="text-xs text-gray-300 font-mono flex-1">{sshCommand}</code>
              <CopyButton text={sshCommand} />
            </div>
          )}
          {sshCommand && tmuxCommand && (
            <div className="flex items-center gap-2">
              <span className="text-xs text-gray-500 w-20">Full:</span>
              <code className="text-xs text-gray-300 font-mono flex-1">{sshCommand} -t {tmuxCommand}</code>
              <CopyButton text={`${sshCommand} -t '${tmuxCommand}'`} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

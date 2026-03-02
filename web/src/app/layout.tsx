import type { Metadata } from 'next';
import './globals.css';
import { LayoutShell } from '@/components/layout-shell';

export const metadata: Metadata = {
  title: 'Awayteam',
  description: 'Monitor and interact with AI agents',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <head>
        <link
          href="https://fonts.googleapis.com/css2?family=Geist:wght@400;500;600;700&family=Geist+Mono:wght@400;500;600&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="font-[Geist] antialiased">
        <LayoutShell>{children}</LayoutShell>
      </body>
    </html>
  );
}

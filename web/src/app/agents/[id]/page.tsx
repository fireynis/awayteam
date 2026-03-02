import AgentPageClient from './client';

export default async function AgentPage({ params }: { params: Promise<{ id: string }> }) {
  return <AgentPageClient params={params} />;
}

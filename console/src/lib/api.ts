import type { Resource, HealthResponse } from '../types/resource';

const ARM_BASE = '/api';
const HEALTH_BASE = '/health';

export async function fetchHealth(): Promise<HealthResponse> {
  const res = await fetch(HEALTH_BASE);
  if (!res.ok) throw new Error(`Health check failed: ${res.status}`);
  return res.json();
}

export async function fetchResources(): Promise<Record<string, Resource>> {
  const res = await fetch(`${ARM_BASE}/state/export`);
  if (!res.ok) throw new Error(`State export failed: ${res.status}`);
  return res.json();
}

export async function importState(data: Record<string, Resource>): Promise<void> {
  const res = await fetch(`${ARM_BASE}/state/import`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`State import failed: ${res.status}`);
}

export async function resetState(): Promise<void> {
  const res = await fetch(`${ARM_BASE}/state/reset`, { method: 'POST' });
  if (!res.ok) throw new Error(`State reset failed: ${res.status}`);
}

export function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

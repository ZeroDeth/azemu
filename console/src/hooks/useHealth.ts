import { useState, useEffect } from 'react';
import type { HealthResponse } from '../types/resource';
import { fetchHealth } from '../lib/api';

export function useHealth(intervalMs = 5000) {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;
    let timerId: ReturnType<typeof setTimeout>;

    async function poll() {
      try {
        const data = await fetchHealth();
        if (mounted) {
          setHealth(data);
          setError(null);
        }
      } catch (err) {
        if (mounted) setError(err instanceof Error ? err.message : 'unreachable');
      }
      // Schedule the next poll only after the current one completes so
      // slow fetches don't cause overlapping concurrent requests.
      if (mounted) {
        timerId = setTimeout(poll, intervalMs);
      }
    }

    poll();
    return () => { mounted = false; clearTimeout(timerId); };
  }, [intervalMs]);

  return { health, error };
}

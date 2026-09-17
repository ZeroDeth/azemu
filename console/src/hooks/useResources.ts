import { useState, useEffect, useCallback, useRef } from 'react';
import type { Resource } from '../types/resource';
import { isAliasResource } from '../types/resource';
import { fetchResources } from '../lib/api';

export function useResources() {
  const [resources, setResources] = useState<Record<string, Resource>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // Refresh can be triggered faster than the server answers. Without a
  // sequence number a slow earlier response overwrites a fast later one.
  const seq = useRef(0);

  const refresh = useCallback(async () => {
    const ticket = ++seq.current;
    try {
      setLoading(true);
      const data = await fetchResources();
      if (ticket !== seq.current) return;
      setResources(data);
      setError(null);
    } catch (err) {
      if (ticket !== seq.current) return;
      setError(err instanceof Error ? err.message : 'fetch failed');
    } finally {
      if (ticket === seq.current) setLoading(false);
    }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  // Alias entries are pointers to a real resource, not resources of their
  // own; including them double-counts every Key Vault key.
  const resourceList = Object.values(resources).filter((r) => !isAliasResource(r));

  return { resources, resourceList, loading, error, refresh };
}

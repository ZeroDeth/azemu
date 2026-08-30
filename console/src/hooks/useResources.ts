import { useState, useEffect, useCallback } from 'react';
import type { Resource } from '../types/resource';
import { isAliasResource } from '../types/resource';
import { fetchResources } from '../lib/api';

export function useResources() {
  const [resources, setResources] = useState<Record<string, Resource>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setLoading(true);
      const data = await fetchResources();
      setResources(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'fetch failed');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  // Alias entries are pointers to a real resource, not resources of their
  // own; including them double-counts every Key Vault key.
  const resourceList = Object.values(resources).filter((r) => !isAliasResource(r));

  return { resources, resourceList, loading, error, refresh };
}

import { useCallback, useState } from 'react';
import type { Resource } from '../types/resource';
import { fetchResources, importState, resetState } from '../lib/api';

/**
 * The export file is whatever the user picked off disk. Anything that is not a
 * map of id to resource object would replace the emulator's store with a nil or
 * zero-valued map, and the next write into it panics, so the shape is checked
 * before it is sent.
 */
function parseStateFile(text: string): Record<string, Resource> {
  const parsed: unknown = JSON.parse(text);
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('Expected a JSON object of resource id to resource.');
  }
  for (const [id, value] of Object.entries(parsed as Record<string, unknown>)) {
    if (value === null || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error(`Entry "${id}" is not a resource object.`);
    }
    const r = value as Partial<Resource>;
    if (typeof r.id !== 'string' || typeof r.type !== 'string') {
      throw new Error(`Entry "${id}" is missing an id or type.`);
    }
  }
  return parsed as Record<string, Resource>;
}

/**
 * Export, import and reset of the emulator's whole state. Lives in a hook
 * because both the overview and the state store view offer the same three
 * actions and they must behave identically in each.
 *
 * Failures surface through `error` rather than the returned promises: every
 * caller drives these straight from a button handler, so a rejection nobody
 * awaits would leave the user believing the action worked.
 */
export function useStateActions(refresh: () => void) {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const run = useCallback(async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }, []);

  const exportState = useCallback(
    () =>
      run(async () => {
        const data = await fetchResources();
        const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'azemu-state.json';
        a.click();
        URL.revokeObjectURL(url);
      }),
    [run],
  );

  const importFromFile = useCallback(() => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = () => {
      const file = input.files?.[0];
      if (!file) return;
      void run(async () => {
        const data = parseStateFile(await file.text());
        const count = Object.keys(data).length;
        // Import replaces the store outright, so it is as destructive as reset.
        if (!confirm(`Replace all emulator state with ${count} resources from ${file.name}?`)) {
          return;
        }
        await importState(data);
        refresh();
      });
    };
    input.click();
  }, [refresh, run]);

  const reset = useCallback(() => {
    if (!confirm('Reset all resources? This cannot be undone.')) return;
    void run(async () => {
      await resetState();
      refresh();
    });
  }, [refresh, run]);

  return { exportState, importFromFile, reset, error, busy, clearError: () => setError(null) };
}

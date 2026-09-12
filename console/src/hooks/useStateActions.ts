import { useCallback } from 'react';
import type { Resource } from '../types/resource';
import { fetchResources, importState, resetState } from '../lib/api';

/**
 * Export, import and reset of the emulator's whole state. Lives in a hook
 * because both the overview and the state store view offer the same three
 * actions and they must behave identically in each.
 */
export function useStateActions(refresh: () => void) {
  const exportState = useCallback(async () => {
    const data = await fetchResources();
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'azemu-state.json';
    a.click();
    URL.revokeObjectURL(url);
  }, []);

  const importFromFile = useCallback(() => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) return;
      const data = JSON.parse(await file.text()) as Record<string, Resource>;
      await importState(data);
      refresh();
    };
    input.click();
  }, [refresh]);

  const reset = useCallback(async () => {
    if (!confirm('Reset all resources? This cannot be undone.')) return;
    await resetState();
    refresh();
  }, [refresh]);

  return { exportState, importFromFile, reset };
}

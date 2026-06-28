import { useNavigate } from 'react-router-dom';
import { useHealth } from '../hooks/useHealth';
import { useResources } from '../hooks/useResources';
import { useRequestLog } from '../hooks/useRequestLog';
import { IconRail } from '../components/IconRail';
import { ServiceCards } from '../components/ServiceCards';
import { MetaStrip } from '../components/MetaStrip';
import { InventoryTiles } from '../components/InventoryTiles';
import { RequestLog } from '../components/RequestLog';
import { fetchResources, resetState, importState } from '../lib/api';
import type { Resource } from '../types/resource';
import styles from './CockpitView.module.css';

const RAIL_ROUTES: Record<string, string> = {
  overview: '/',
  resources: '/explorer',
};

export function CockpitView() {
  const navigate = useNavigate();
  const { health, error: healthError } = useHealth();
  const { resourceList, refresh } = useResources();
  const { entries: logEntries } = useRequestLog();

  const handleExport = async () => {
    try {
      const data = await fetchResources();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'azemu-state.json';
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      console.error('Export failed:', err);
    }
  };

  const handleImport = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = async () => {
      try {
        const file = input.files?.[0];
        if (!file) return;
        const text = await file.text();
        const data = JSON.parse(text) as Record<string, Resource>;
        await importState(data);
        refresh();
      } catch (err) {
        console.error('Import failed:', err);
      }
    };
    input.click();
  };

  const handleReset = async () => {
    if (!confirm('Reset all resources? This cannot be undone.')) return;
    try {
      await resetState();
      refresh();
    } catch (err) {
      console.error('Reset failed:', err);
    }
  };

  const startTime = health
    ? new Date(Date.now() - health.uptime_seconds * 1000)
        .toLocaleTimeString('en-GB', { hour12: false })
    : '...';

  return (
    <>
      <IconRail onSelect={(id) => { const r = RAIL_ROUTES[id]; if (r) navigate(r); }} />
      <div className={styles.dashboard}>
        <div className={styles.headerRow}>
          <h1 className={styles.heading}>Overview</h1>
          <span className={styles.meta}>
            emulator started {startTime} ·{' '}
            <span className={healthError ? styles.unhealthy : styles.healthy}>
              {healthError ? 'unreachable' : 'healthy'}
            </span>
          </span>
        </div>

        <ServiceCards healthy={!healthError} />
        <MetaStrip health={health} resourceCount={resourceList.length} />

        <div className={styles.bottomGrid}>
          <InventoryTiles
            resources={resourceList}
            onExport={handleExport}
            onImport={handleImport}
            onReset={handleReset}
          />
          <RequestLog entries={logEntries} />
        </div>
      </div>
    </>
  );
}

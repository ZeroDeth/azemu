import { useNavigate } from 'react-router-dom';
import { useHealth } from '../hooks/useHealth';
import { useResources } from '../hooks/useResources';
import { useRequestLog } from '../hooks/useRequestLog';
import { IconRail } from '../components/IconRail';
import { ServiceCards } from '../components/ServiceCards';
import { MetaStrip } from '../components/MetaStrip';
import { InventoryTiles } from '../components/InventoryTiles';
import { RequestLog } from '../components/RequestLog';
import { useStateActions } from '../hooks/useStateActions';
import styles from './CockpitView.module.css';

const RAIL_ROUTES: Record<string, string> = {
  overview: '/',
  resources: '/explorer',
  networking: '/networking',
  keyvault: '/key-vault',
  storage: '/storage',
  health: '/emulator/health',
  'state-store': '/emulator/state-store',
};

export function CockpitView() {
  const navigate = useNavigate();
  const { health, error: healthError } = useHealth();
  const { resourceList, refresh } = useResources();
  const { exportState, importFromFile, reset, error: actionError } = useStateActions(refresh);
  const { entries: logEntries } = useRequestLog();

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

        <ServiceCards
          state={healthError ? 'unreachable' : health ? 'healthy' : 'checking'}
          armRequests={logEntries.length}
        />
        <MetaStrip health={health} resourceCount={resourceList.length} />

        {actionError && (
          <div className={styles.actionError} role="alert">
            {actionError}
          </div>
        )}

        <div className={styles.bottomGrid}>
          <InventoryTiles
            resources={resourceList}
            onExport={exportState}
            onImport={importFromFile}
            onReset={reset}
          />
          <RequestLog entries={logEntries} />
        </div>
      </div>
    </>
  );
}

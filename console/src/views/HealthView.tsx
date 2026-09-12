import { PageShell } from './PageShell';
import { ServiceCards } from '../components/ServiceCards';
import { MetaStrip } from '../components/MetaStrip';
import { StatusDot } from '../components/StatusDot';
import { useHealth } from '../hooks/useHealth';
import { useResources } from '../hooks/useResources';
import { useRequestLog } from '../hooks/useRequestLog';
import styles from './HealthView.module.css';

export function HealthView() {
  const { health, error } = useHealth();
  const { resourceList } = useResources();
  const { entries, connected } = useRequestLog();

  return (
    <PageShell
      active="health"
      title="Health"
      subtitle="Reported by the emulator, not inferred by this page"
      aside={
        <>
          <StatusDot color={error ? '#f85149' : '#3fb950'} glow />
          {error ? 'Unreachable' : 'Healthy'}
        </>
      }
    >
      <MetaStrip health={health} resourceCount={resourceList.length} />

      <h2 className={styles.heading}>Listeners</h2>
      <ServiceCards healthy={!error} armRequests={entries.length} />

      <h2 className={styles.heading}>Request log stream</h2>
      <div className={styles.streamRow}>
        <StatusDot color={connected ? '#3fb950' : '#d29922'} glow />
        {connected
          ? `Connected. ${entries.length} requests observed this session.`
          : 'Disconnected. The console reconnects automatically.'}
      </div>

      {error && (
        <div className={styles.error}>
          <div className={styles.errorTitle}>Health check failed</div>
          <div className={styles.errorBody}>{error}</div>
        </div>
      )}
    </PageShell>
  );
}

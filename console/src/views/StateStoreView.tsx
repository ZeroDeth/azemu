import { useMemo } from 'react';
import { Download, Upload, RotateCcw, AlertTriangle } from 'lucide-react';
import { PageShell } from './PageShell';
import { CategoryBadge } from '../components/CategoryBadge';
import { useResources } from '../hooks/useResources';
import { useHealth } from '../hooks/useHealth';
import { useStateActions } from '../hooks/useStateActions';
import { getCategoryForType } from '../types/resource';
import styles from './StateStoreView.module.css';

export function StateStoreView() {
  const { resources, resourceList, refresh } = useResources();
  const { health } = useHealth();
  const { exportState, importFromFile, reset, error, busy } = useStateActions(refresh);

  const byType = useMemo(() => {
    const counts = new Map<string, number>();
    for (const r of resourceList) counts.set(r.type, (counts.get(r.type) ?? 0) + 1);
    return [...counts.entries()].sort((a, b) => b[1] - a[1]);
  }, [resourceList]);

  const durable = health?.store === 'file-backed';

  return (
    <PageShell
      active="state-store"
      title="State store"
      subtitle={health ? `${health.store} · ${resourceList.length} resources` : '...'}
      aside={`${Object.keys(resources).length} entries including aliases`}
    >
      {health && !durable && (
        <div className={styles.warning}>
          <AlertTriangle size={14} strokeWidth={1.7} className={styles.warningIcon} />
          <div>
            This emulator is running the in-memory store. Everything here is lost
            on restart, and Terraform state will then point at resources that no
            longer exist. Set AZEMU_PERSIST_PATH to keep it on disk.
          </div>
        </div>
      )}

      <div className={styles.actions}>
        <button className={styles.btn} onClick={exportState} disabled={busy}>
          <Download size={13} strokeWidth={1.7} /> Export
        </button>
        <button className={styles.btn} onClick={importFromFile} disabled={busy}>
          <Upload size={13} strokeWidth={1.7} /> Import
        </button>
        <button className={`${styles.btn} ${styles.danger}`} onClick={reset} disabled={busy}>
          <RotateCcw size={13} strokeWidth={1.7} /> Reset
        </button>
      </div>

      {error && (
        <div className={styles.actionError}>
          <AlertTriangle size={14} strokeWidth={1.7} className={styles.warningIcon} />
          <div>{error}</div>
        </div>
      )}

      {byType.length === 0 ? (
        <div className={styles.empty}>
          The store is empty. Run terraform apply against azemu to populate it.
        </div>
      ) : (
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Resource type</th>
              <th className={styles.numCol}>Count</th>
            </tr>
          </thead>
          <tbody>
            {byType.map(([type, count]) => {
              const cat = getCategoryForType(type);
              return (
                <tr key={type}>
                  <td className={styles.nameCell}>
                    {cat && <CategoryBadge code={cat.code} size={18} />}
                    {type}
                  </td>
                  <td className={styles.numCol}>{count}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </PageShell>
  );
}

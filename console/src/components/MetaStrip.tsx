import type { HealthResponse } from '../types/resource';
import { formatUptime } from '../lib/api';
import styles from './MetaStrip.module.css';

interface Props {
  health: HealthResponse | null;
  resourceCount: number;
}

export function MetaStrip({ health, resourceCount }: Props) {
  const items = [
    { label: 'Version', value: health?.version ?? '...', accent: true },
    { label: 'Uptime', value: health ? formatUptime(health.uptime_seconds) : '...' },
    { label: 'Resources', value: String(resourceCount) },
    { label: 'Store', value: 'file-backed' },
    { label: 'TLS', value: 'ECDSA P-256' },
  ];

  return (
    <div className={styles.grid}>
      {items.map((item) => (
        <div key={item.label} className={styles.card}>
          <div className={styles.label}>{item.label}</div>
          <div className={`${styles.value} ${item.accent ? styles.accent : ''}`}>
            {item.value}
          </div>
        </div>
      ))}
    </div>
  );
}

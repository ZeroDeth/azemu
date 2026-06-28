import { StatusDot } from './StatusDot';
import { METHOD_COLORS, statusColor } from '../types/resource';
import styles from './RequestLog.module.css';

interface LogEntry {
  ts: string;
  method: string;
  path: string;
  status: number;
  durationMs: number;
}

interface Props {
  entries?: LogEntry[];
  compact?: boolean;
}

export function RequestLog({ entries = [] }: Props) {
  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <StatusDot color="#3fb950" glow size={7} />
          <span className={styles.headerTitle}>Live request log</span>
        </div>
        <span className={styles.headerMeta}>tail · ARM :4566</span>
      </div>
      <div className={styles.rows}>
        {entries.length === 0 ? (
          <div className={styles.empty ?? ''} style={{ padding: '12px 16px', color: '#484f58', fontSize: 12 }}>
            No requests yet — run <code>terraform apply</code> against azemu
          </div>
        ) : (
          entries.map((l, i) => (
            <div key={i} className={styles.row}>
              <span className={styles.ts}>{l.ts}</span>
              <span
                className={styles.method}
                style={{ color: METHOD_COLORS[l.method] ?? '#8b949e' }}
              >
                {l.method}
              </span>
              <span className={styles.path}>{l.path}</span>
              <span
                className={styles.status}
                style={{ color: statusColor(l.status) }}
              >
                {l.status}
              </span>
              <span className={styles.dur}>{l.durationMs}ms</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

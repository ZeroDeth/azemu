import { StatusDot } from './StatusDot';
import { METHOD_COLORS, statusColor } from '../types/resource';
import styles from './DockedLog.module.css';

interface LogEntry {
  ts: string;
  method: string;
  path: string;
  status: number;
  durationMs: number;
}

interface Props {
  entries?: LogEntry[];
}

export function DockedLog({ entries = [] }: Props) {
  const errorCount = entries.filter((e) => e.status >= 400).length;

  return (
    <div className={styles.dock}>
      <div className={styles.tabBar}>
        <div className={styles.tabActive}>
          <StatusDot color="#3fb950" glow size={7} />
          Request log
        </div>
        <div className={styles.tab}>Events</div>
        <div className={styles.tab}>State store</div>
        <div className={styles.spacer} />
        <span className={styles.stats}>
          {entries.length} requests · {errorCount} errors
        </span>
      </div>
      <div className={styles.rows}>
        {entries.length === 0 ? (
          <div style={{ padding: '8px 16px', color: '#484f58', fontSize: 12 }}>
            No requests yet
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

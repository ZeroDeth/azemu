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

const MOCK_LOG: LogEntry[] = [
  { ts: '15:42:18.204', method: 'PUT', path: '/subscriptions/.../resourceGroups/rg-platform', status: 201, durationMs: 12 },
  { ts: '15:42:18.231', method: 'GET', path: '/metadata/endpoints', status: 200, durationMs: 3 },
  { ts: '15:42:18.402', method: 'POST', path: '/{tenant}/oauth2/v2.0/token', status: 200, durationMs: 8 },
  { ts: '15:42:19.118', method: 'PUT', path: '.../Microsoft.Network/virtualNetworks/vnet-core', status: 201, durationMs: 15 },
  { ts: '15:42:19.260', method: 'PUT', path: '.../virtualNetworks/vnet-core/subnets/snet-app', status: 200, durationMs: 9 },
  { ts: '15:42:19.388', method: 'PUT', path: '.../virtualNetworks/vnet-core/subnets/snet-data', status: 200, durationMs: 7 },
  { ts: '15:42:20.041', method: 'PUT', path: '.../networkSecurityGroups/nsg-app', status: 201, durationMs: 11 },
  { ts: '15:42:20.515', method: 'PUT', path: '.../Microsoft.KeyVault/vaults/kv-platform', status: 200, durationMs: 18 },
  { ts: '15:42:21.002', method: 'GET', path: '.../Microsoft.KeyVault/vaults/kv-platform', status: 200, durationMs: 4 },
  { ts: '15:42:21.244', method: 'DELETE', path: '.../resourceGroups/rg-staging', status: 202, durationMs: 22 },
  { ts: '15:42:21.870', method: 'HEAD', path: '.../publicIPAddresses/pip-gw', status: 200, durationMs: 2 },
];

interface Props {
  entries?: LogEntry[];
  compact?: boolean;
}

export function RequestLog({ entries = MOCK_LOG }: Props) {
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
        {entries.map((l, i) => (
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
        ))}
      </div>
    </div>
  );
}

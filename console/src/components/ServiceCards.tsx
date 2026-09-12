import { StatusDot } from './StatusDot';
import styles from './ServiceCards.module.css';

interface Service {
  name: string;
  port: string;
  proto: string;
  /** True when the request log observes this port, so a count can be shown. */
  metered: boolean;
}

const SERVICES: Service[] = [
  { name: 'ARM management API', port: '4566', proto: 'HTTPS', metered: true },
  { name: 'Metadata · OAuth2 · OIDC', port: '4567', proto: 'HTTPS', metered: false },
  { name: 'Health probe', port: '4568', proto: 'HTTP', metered: false },
  { name: 'Azure DevOps OIDC', port: '4569', proto: 'HTTP', metered: false },
];

interface Props {
  healthy: boolean;
  /** Requests seen on the ARM port this session, from the live request log. */
  armRequests: number;
}

export function ServiceCards({ healthy, armRequests }: Props) {
  return (
    <div className={styles.grid}>
      {SERVICES.map((s) => (
        <div key={s.port} className={styles.card}>
          <div className={styles.cardTop}>
            <span className={styles.status}>
              <StatusDot color={healthy ? '#3fb950' : '#f85149'} glow />
              {healthy ? 'Running' : 'Unreachable'}
            </span>
            <span className={styles.port}>:{s.port}</span>
          </div>
          <div className={styles.name}>{s.name}</div>
          <div className={styles.meta}>
            <span>{s.proto}</span>
            {/* Only the ARM port is instrumented; the others would be a
                fabricated number, so they show nothing rather than "0 req". */}
            <span>{s.metered ? `${armRequests} req` : 'not metered'}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

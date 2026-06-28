import { StatusDot } from './StatusDot';
import styles from './ServiceCards.module.css';

interface Service {
  name: string;
  port: string;
  proto: string;
  reqs: string;
  status: string;
}

const SERVICES: Service[] = [
  { name: 'ARM management API', port: '4566', proto: 'HTTPS', reqs: '0', status: 'Running' },
  { name: 'Metadata · OAuth2 · OIDC', port: '4567', proto: 'HTTPS', reqs: '0', status: 'Running' },
  { name: 'Health probe', port: '4568', proto: 'HTTP', reqs: '0', status: 'Running' },
  { name: 'Azure DevOps OIDC', port: '4569', proto: 'HTTP', reqs: '0', status: 'Running' },
];

interface Props {
  healthy: boolean;
}

export function ServiceCards({ healthy }: Props) {
  return (
    <div className={styles.grid}>
      {SERVICES.map((s) => (
        <div key={s.port} className={styles.card}>
          <div className={styles.cardTop}>
            <span className={styles.status}>
              <StatusDot color={healthy ? '#3fb950' : '#f85149'} glow />
              {healthy ? s.status : 'Unreachable'}
            </span>
            <span className={styles.port}>:{s.port}</span>
          </div>
          <div className={styles.name}>{s.name}</div>
          <div className={styles.meta}>
            <span>{s.proto}</span>
            <span>{s.reqs} req</span>
          </div>
        </div>
      ))}
    </div>
  );
}

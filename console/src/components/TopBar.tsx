import { Search, Bell, Settings } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useHealth } from '../hooks/useHealth';
import { StatusDot } from './StatusDot';
import styles from './TopBar.module.css';

export function TopBar() {
  const navigate = useNavigate();
  const { error } = useHealth();

  return (
    <header className={styles.bar}>
      <button className={styles.logo} onClick={() => navigate('/')} aria-label="Go to overview">
        <div className={styles.logoIcon}>az</div>
        <div className={styles.logoText}>
          <span className={styles.wordmark}>azemu</span>
          <span className={styles.kicker}>CONSOLE</span>
        </div>
      </button>

      <div className={styles.search}>
        <Search size={14} color="#484f58" strokeWidth={1.6} />
        <span className={styles.searchPlaceholder}>
          Search resources, services, docs...
        </span>
      </div>

      <div className={styles.spacer} />

      <div className={styles.envPill}>
        <StatusDot color={error ? '#f85149' : '#3fb950'} glow />
        <span className={styles.envName}>azemu-local</span>
        <span className={styles.envDivider} />
        <span className={styles.envRegion}>westeurope</span>
      </div>

      <button className={styles.iconBtn} aria-label="Notifications">
        <Bell size={15} strokeWidth={1.5} />
      </button>
      <button className={styles.iconBtn} aria-label="Settings">
        <Settings size={15} strokeWidth={1.5} />
      </button>
    </header>
  );
}

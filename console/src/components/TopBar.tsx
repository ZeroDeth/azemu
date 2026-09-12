import { useState } from 'react';
import { Search } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useHealth } from '../hooks/useHealth';
import { StatusDot } from './StatusDot';
import styles from './TopBar.module.css';

export function TopBar() {
  const navigate = useNavigate();
  const { health, error } = useHealth();
  const [query, setQuery] = useState('');

  // Enter runs the search in the explorer, which owns the resource list.
  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key !== 'Enter') return;
    const q = query.trim();
    navigate(q ? `/explorer?q=${encodeURIComponent(q)}` : '/explorer');
  }

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
        <input
          className={styles.searchInput}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Search resources by name, type or group..."
          aria-label="Search resources"
        />
      </div>

      <div className={styles.spacer} />

      <div className={styles.envPill}>
        <StatusDot color={error ? '#f85149' : '#3fb950'} glow />
        <span className={styles.envName}>azemu-local</span>
        <span className={styles.envDivider} />
        <span className={styles.envRegion}>{health?.version ?? '...'}</span>
      </div>
    </header>
  );
}

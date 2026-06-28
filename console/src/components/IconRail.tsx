import {
  LayoutGrid, Box, Network, Shield, Database, Activity, Archive,
} from 'lucide-react';
import styles from './IconRail.module.css';

// Items with a route are navigable; those without are rendered disabled.
const ITEMS = [
  { icon: LayoutGrid, id: 'overview', route: true },
  { icon: Box, id: 'resources', route: true },
  { icon: Network, id: 'networking', route: false },
  { icon: Shield, id: 'keyvault', route: false },
  { icon: Database, id: 'storage', route: false },
  { icon: Activity, id: 'health', route: false },
] as const;

interface Props {
  active?: string;
  onSelect?: (id: string) => void;
}

export function IconRail({ active = 'overview', onSelect }: Props) {
  return (
    <nav className={styles.rail}>
      {ITEMS.map(({ icon: Icon, id, route }) => (
        <button
          key={id}
          className={`${styles.btn} ${active === id ? styles.active : ''}`}
          onClick={() => route && onSelect?.(id)}
          aria-label={id}
          disabled={!route}
          title={route ? undefined : 'Not yet implemented'}
        >
          <Icon size={17} strokeWidth={1.6} />
        </button>
      ))}
      <div className={styles.spacer} />
      <button className={styles.btn} aria-label="State store" disabled title="Not yet implemented">
        <Archive size={17} strokeWidth={1.6} />
      </button>
    </nav>
  );
}

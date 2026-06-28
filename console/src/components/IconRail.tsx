import {
  LayoutGrid, Box, Network, Shield, Database, Activity, Archive,
} from 'lucide-react';
import styles from './IconRail.module.css';

const ITEMS = [
  { icon: LayoutGrid, id: 'overview', active: true },
  { icon: Box, id: 'resources' },
  { icon: Network, id: 'networking' },
  { icon: Shield, id: 'keyvault' },
  { icon: Database, id: 'storage' },
  { icon: Activity, id: 'health' },
] as const;

interface Props {
  active?: string;
  onSelect?: (id: string) => void;
}

export function IconRail({ active = 'overview', onSelect }: Props) {
  return (
    <nav className={styles.rail}>
      {ITEMS.map(({ icon: Icon, id }) => (
        <button
          key={id}
          className={`${styles.btn} ${active === id ? styles.active : ''}`}
          onClick={() => onSelect?.(id)}
          aria-label={id}
        >
          <Icon size={17} strokeWidth={1.6} />
        </button>
      ))}
      <div className={styles.spacer} />
      <button className={styles.btn} aria-label="State store">
        <Archive size={17} strokeWidth={1.6} />
      </button>
    </nav>
  );
}

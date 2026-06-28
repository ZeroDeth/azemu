import {
  Home, Box, LayoutGrid, Network, Database, Shield, Globe,
  Cpu, Activity, List, Archive,
} from 'lucide-react';
import styles from './SideNav.module.css';

export interface NavItem {
  id: string;
  label: string;
  icon: React.ComponentType<{ size?: number; strokeWidth?: number }>;
  section?: string;
}

const NAV_ITEMS: NavItem[] = [
  { id: 'overview', label: 'Overview', icon: Home },
  { id: 'resource-groups', label: 'Resource groups', icon: Box },
  { id: 'all-resources', label: 'All resources', icon: LayoutGrid },
  { id: 'networking', label: 'Networking', icon: Network, section: 'SERVICES' },
  { id: 'storage', label: 'Storage', icon: Database },
  { id: 'key-vault', label: 'Key Vault', icon: Shield },
  { id: 'dns-zones', label: 'DNS zones', icon: Globe },
  { id: 'databases', label: 'Databases', icon: Cpu },
  { id: 'health', label: 'Health', icon: Activity, section: 'EMULATOR' },
  { id: 'request-log', label: 'Request log', icon: List },
  { id: 'state-store', label: 'State store', icon: Archive },
];

interface Props {
  active?: string;
  onSelect?: (id: string) => void;
  compact?: boolean;
  width?: number;
}

export function SideNav({ active = 'overview', onSelect, compact = false, width = 224 }: Props) {
  return (
    <nav className={styles.nav} style={{ width, flex: `0 0 ${width}px` }}>
      {NAV_ITEMS.map((item) => (
        <div key={item.id}>
          {item.section && (
            <div className={styles.section}>{item.section}</div>
          )}
          <button
            className={`${styles.item} ${active === item.id ? styles.active : ''}`}
            onClick={() => onSelect?.(item.id)}
            style={{ fontSize: compact ? 11.5 : 12 }}
          >
            <item.icon size={compact ? 15 : 16} strokeWidth={1.6} />
            {item.label}
          </button>
        </div>
      ))}
    </nav>
  );
}

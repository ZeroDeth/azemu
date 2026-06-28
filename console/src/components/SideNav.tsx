import {
  Home, Box, LayoutGrid, Network, Database, Shield, Globe,
  Cpu, Activity, List, Archive,
} from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import styles from './SideNav.module.css';

export interface NavItem {
  id: string;
  label: string;
  icon: React.ComponentType<{ size?: number; strokeWidth?: number }>;
  section?: string;
  route?: string;
}

// Only items with a route are navigable; others render disabled.
const NAV_ITEMS: NavItem[] = [
  { id: 'overview', label: 'Overview', icon: Home, route: '/' },
  { id: 'resource-groups', label: 'Resource groups', icon: Box, route: '/' },
  { id: 'all-resources', label: 'All resources', icon: LayoutGrid, route: '/explorer' },
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
  const navigate = useNavigate();

  function handleClick(item: NavItem) {
    if (item.route) {
      navigate(item.route);
    }
    onSelect?.(item.id);
  }

  return (
    <nav className={styles.nav} style={{ width, flex: `0 0 ${width}px` }}>
      {NAV_ITEMS.map((item) => (
        <div key={item.id}>
          {item.section && (
            <div className={styles.section}>{item.section}</div>
          )}
          <button
            className={`${styles.item} ${active === item.id ? styles.active : ''}`}
            onClick={() => handleClick(item)}
            disabled={!item.route}
            title={item.route ? undefined : 'Not yet implemented'}
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

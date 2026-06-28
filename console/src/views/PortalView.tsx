import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { SideNav } from '../components/SideNav';
import { CategoryBadge } from '../components/CategoryBadge';
import { StatusDot } from '../components/StatusDot';
import { useResources } from '../hooks/useResources';
import { getResourceGroup, getCategoryForType } from '../types/resource';
import type { Resource } from '../types/resource';
import { Plus, RefreshCw, Download, Trash2, Search } from 'lucide-react';
import styles from './PortalView.module.css';

interface Props {
  resourceGroupName?: string;
}

export function PortalView({ resourceGroupName = 'rg-platform' }: Props) {
  const { resourceList } = useResources();

  const rgResources = useMemo(
    () => resourceList.filter((r) => {
      const rg = getResourceGroup(r.id);
      return rg?.toLowerCase() === resourceGroupName.toLowerCase()
        && r.type !== 'Microsoft.Resources/resourceGroups';
    }),
    [resourceList, resourceGroupName],
  );

  return (
    <>
      <SideNav active="resource-groups" />
      <div className={styles.main}>
        {/* Breadcrumb */}
        <div className={styles.breadcrumb}>
          <Link to="/" className={styles.breadcrumbLink}>Home</Link>
          <span className={styles.breadcrumbSep}>/</span>
          <Link to="/" className={styles.breadcrumbLink}>Resource groups</Link>
          <span className={styles.breadcrumbSep}>/</span>
          <span className={styles.breadcrumbCurrent}>{resourceGroupName}</span>
        </div>

        {/* Header */}
        <div className={styles.header}>
          <CategoryBadge code="RG" size={36} />
          <div className={styles.headerText}>
            <h1 className={styles.title}>{resourceGroupName}</h1>
            <div className={styles.subtitle}>
              Resource group · subscription{' '}
              <span className={styles.subMono}>azemu-local</span>
            </div>
          </div>
        </div>

        {/* Command bar */}
        <div className={styles.commandBar}>
          <button className={styles.primaryBtn} disabled title="Not yet implemented">
            <Plus size={13} strokeWidth={2} />
            Create
          </button>
          <button className={styles.secondaryBtn}>
            <RefreshCw size={13} strokeWidth={1.6} />
            Refresh
          </button>
          <button className={styles.secondaryBtn} disabled title="Not yet implemented">
            <Download size={13} strokeWidth={1.6} />
            Export template
          </button>
          <button className={styles.deleteBtn} disabled title="Not yet implemented">
            <Trash2 size={13} strokeWidth={1.6} />
            Delete
          </button>
        </div>

        {/* Essentials */}
        <div className={styles.essentials}>
          <EssentialCell label="Subscription" value="azemu-local" />
          <EssentialCell label="Subscription ID" value="00000000-0000-0000-0000-000000000000" mono link />
          <EssentialCell label="Location" value="West Europe" />
          <EssentialCell label="Provisioning state" value="Succeeded" status />
          <EssentialCell label="Resources" value={`${rgResources.length} objects`} />
          <EssentialCell label="Last deployment" terraform />
        </div>

        {/* Resources table */}
        <div className={styles.tableHeader}>
          <span className={styles.tableTitle}>
            Resources <span className={styles.tableCount}>({rgResources.length})</span>
          </span>
          <div className={styles.filterChip}>
            <Search size={12} strokeWidth={1.6} color="#484f58" />
            <span>Filter by name...</span>
          </div>
        </div>

        <div className={styles.table}>
          <div className={styles.tableHead}>
            <span>Name</span>
            <span>Type</span>
            <span>Location</span>
            <span>Status</span>
            <span>Created</span>
          </div>
          <div className={styles.tableBody}>
            {rgResources.length === 0 ? (
              <div className={styles.emptyRow}>
                No resources yet — run <code>terraform apply</code> against azemu
              </div>
            ) : (
              rgResources.map((r) => <ResourceRow key={r.id} resource={r} />)
            )}
          </div>
        </div>
      </div>
    </>
  );
}

function ResourceRow({ resource }: { resource: Resource }) {
  const cat = getCategoryForType(resource.type);
  const typeName = resource.type.split('/').pop() ?? resource.type;
  const ago = timeAgo(resource.createdAt);

  return (
    <div className={styles.row}>
      <span className={styles.nameCell}>
        {cat && <CategoryBadge code={cat.code} size={22} />}
        <span className={styles.resourceName}>{resource.name}</span>
      </span>
      <span className={styles.typeCell}>{typeName}</span>
      <span className={styles.locCell}>{resource.location}</span>
      <span className={styles.statusCell}>
        <StatusDot color="#3fb950" size={6} />
        Succeeded
      </span>
      <span className={styles.createdCell}>{ago}</span>
    </div>
  );
}

function EssentialCell({
  label, value, mono, link, status, terraform,
}: {
  label: string;
  value?: string;
  mono?: boolean;
  link?: boolean;
  status?: boolean;
  terraform?: boolean;
}) {
  return (
    <div>
      <div className={styles.essLabel}>{label}</div>
      {terraform ? (
        <div className={styles.essValue}>
          <span className={styles.essTerraform}>terraform apply</span> · 2h ago
        </div>
      ) : status ? (
        <div className={`${styles.essValue} ${styles.essStatus}`}>
          <StatusDot color="#3fb950" size={7} />
          {value}
        </div>
      ) : (
        <div
          className={styles.essValue}
          style={{
            fontFamily: mono ? 'var(--font-mono)' : undefined,
            fontSize: mono ? 11.5 : undefined,
            color: link ? 'var(--link-blue)' : undefined,
          }}
        >
          {value}
        </div>
      )}
    </div>
  );
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

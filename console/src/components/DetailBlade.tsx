import { useState } from 'react';
import { CategoryBadge } from './CategoryBadge';
import { StatusDot } from './StatusDot';
import type { Resource, CategoryCode } from '../types/resource';
import { getCategoryForType, resolveResourceGroup } from '../types/resource';
import styles from './DetailBlade.module.css';

const TABS = ['Overview', 'JSON'] as const;

interface Props {
  resource: Resource;
  /** Full resource list, needed to resolve data-plane ids to their group. */
  all: Resource[];
}

export function DetailBlade({ resource, all }: Props) {
  const [activeTab, setActiveTab] = useState<string>('Overview');
  const cat = getCategoryForType(resource.type);
  const rg = resolveResourceGroup(resource, all);
  const typeName = resource.type.split('/').pop() ?? resource.type;

  // Only ARM resources carry a provisioning state; data-plane objects such as
  // Key Vault keys have none, so the badge is omitted rather than invented.
  const provisioningState = resource.properties?.provisioningState;
  const state = typeof provisioningState === 'string' ? provisioningState : null;

  return (
    <div className={styles.blade}>
      {/* Mini breadcrumb */}
      <div className={styles.miniBreadcrumb}>
        {rg && (
          <>
            <span className={styles.bcLink}>{rg}</span>
            <span className={styles.bcSep}>/</span>
          </>
        )}
        <span className={styles.bcCurrent}>{resource.name}</span>
      </div>

      {/* Header */}
      <div className={styles.header}>
        {cat && <CategoryBadge code={cat.code as CategoryCode} size={34} />}
        <div className={styles.headerText}>
          <h2 className={styles.title}>{resource.name}</h2>
          <div className={styles.subtitle}>
            {typeName}
            {resource.location && ` · ${resource.location}`}
          </div>
        </div>
        {state && (
          <div className={styles.statusBadge}>
            <StatusDot color={state === 'Succeeded' ? '#3fb950' : '#d29922'} size={7} />
            {state}
          </div>
        )}
      </div>

      {/* Tabs */}
      <div className={styles.tabs}>
        {TABS.map((tab) => (
          <button
            key={tab}
            className={`${styles.tab} ${activeTab === tab ? styles.tabActive : ''}`}
            onClick={() => setActiveTab(tab)}
          >
            {tab}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {activeTab === 'JSON' ? (
        <div className={styles.jsonPane}>
          <pre className={styles.jsonPre}>
            {JSON.stringify(resource, null, 2)}
          </pre>
        </div>
      ) : (
        <OverviewTab resource={resource} />
      )}
    </div>
  );
}

function OverviewTab({ resource }: { resource: Resource }) {
  const entries = Object.entries(resource.properties ?? {});

  return (
    <div className={styles.essentials}>
      <div className={styles.essRow}>
        <span className={styles.essKey}>Type</span>
        <span className={styles.essVal}>{resource.type}</span>
      </div>
      {entries.map(([key, val]) => (
        <div key={key} className={styles.essRow}>
          <span className={styles.essKey}>{key}</span>
          <span className={styles.essVal}>
            {typeof val === 'object' ? JSON.stringify(val) : String(val)}
          </span>
        </div>
      ))}
    </div>
  );
}

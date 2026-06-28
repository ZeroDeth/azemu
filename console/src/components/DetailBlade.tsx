import { useState } from 'react';
import { CategoryBadge } from './CategoryBadge';
import { StatusDot } from './StatusDot';
import type { Resource, CategoryCode } from '../types/resource';
import { getCategoryForType, getResourceGroup } from '../types/resource';
import styles from './DetailBlade.module.css';

const TABS = ['Overview', 'Secrets', 'Keys', 'Access', 'JSON'] as const;

interface Props {
  resource: Resource;
}

export function DetailBlade({ resource }: Props) {
  const [activeTab, setActiveTab] = useState<string>('Overview');
  const cat = getCategoryForType(resource.type);
  const rg = getResourceGroup(resource.id);
  const typeName = resource.type.split('/').pop() ?? resource.type;

  return (
    <div className={styles.blade}>
      {/* Mini breadcrumb */}
      <div className={styles.miniBreadcrumb}>
        <span className={styles.bcLink}>{rg}</span>
        <span className={styles.bcSep}>/</span>
        <span className={styles.bcCurrent}>{resource.name}</span>
      </div>

      {/* Header */}
      <div className={styles.header}>
        {cat && <CategoryBadge code={cat.code as CategoryCode} size={34} />}
        <div className={styles.headerText}>
          <h2 className={styles.title}>{resource.name}</h2>
          <div className={styles.subtitle}>
            {typeName} · {resource.location}
          </div>
        </div>
        <div className={styles.statusBadge}>
          <StatusDot color="#3fb950" size={7} />
          Succeeded
        </div>
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
      ) : activeTab === 'Overview' ? (
        <OverviewTab resource={resource} />
      ) : (
        <div className={styles.tabPlaceholder}>
          {activeTab} tab — coming soon
        </div>
      )}
    </div>
  );
}

function OverviewTab({ resource }: { resource: Resource }) {
  const props = resource.properties ?? {};
  const entries = Object.entries(props).slice(0, 8);

  return (
    <>
      <div className={styles.essentials}>
        {entries.map(([key, val]) => (
          <div key={key} className={styles.essRow}>
            <span className={styles.essKey}>{key}</span>
            <span className={styles.essVal}>
              {typeof val === 'object' ? JSON.stringify(val) : String(val)}
            </span>
          </div>
        ))}
        {entries.length === 0 && (
          <div className={styles.essRow}>
            <span className={styles.essKey}>Type</span>
            <span className={styles.essVal}>{resource.type}</span>
          </div>
        )}
      </div>
    </>
  );
}

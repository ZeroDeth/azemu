import { useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { PageShell } from './PageShell';
import { CategoryBadge } from '../components/CategoryBadge';
import { useResources } from '../hooks/useResources';
import {
  getCategoryForType,
  resolveResourceGroup,
  TYPE_TO_CATEGORY,
} from '../types/resource';
import type { CategoryCode } from '../types/resource';
import styles from './ServiceView.module.css';

export interface ServiceDef {
  /** Nav item id, so the side nav highlights the right row. */
  navId: string;
  title: string;
  /** ARM categories that belong to this service area. */
  codes: CategoryCode[];
}

export const SERVICE_VIEWS: Record<string, ServiceDef> = {
  networking: {
    navId: 'networking',
    title: 'Networking',
    codes: ['VN', 'SN', 'NS', 'PI', 'LB', 'AG', 'CD'],
  },
  storage: { navId: 'storage', title: 'Storage', codes: ['ST'] },
  'key-vault': { navId: 'key-vault', title: 'Key Vault', codes: ['KV'] },
  'dns-zones': { navId: 'dns-zones', title: 'DNS zones', codes: ['DN'] },
  databases: { navId: 'databases', title: 'Databases', codes: ['RC', 'AK'] },
};

/** Human-readable list of the ARM types a service view covers, for empty state. */
function typeLabels(codes: CategoryCode[]): string {
  const labels = Object.values(TYPE_TO_CATEGORY)
    .filter((entry) => codes.includes(entry.code))
    .map((entry) => entry.label);
  return [...new Set(labels)].join(', ');
}

export function ServiceView({ service }: { service: ServiceDef }) {
  const { resourceList } = useResources();
  const navigate = useNavigate();

  const subtitle = typeLabels(service.codes);

  const matches = useMemo(
    () =>
      resourceList.filter((r) => {
        const cat = getCategoryForType(r.type);
        return cat ? service.codes.includes(cat.code) : false;
      }),
    [resourceList, service.codes],
  );

  return (
    <PageShell
      active={service.navId}
      title={service.title}
      subtitle={subtitle === service.title ? undefined : subtitle}
      aside={`${matches.length} of ${resourceList.length} resources`}
    >
      {matches.length === 0 ? (
        <div className={styles.empty}>
          <div className={styles.emptyTitle}>No {service.title} resources yet</div>
          <div className={styles.emptyBody}>
            azemu has no resources of this kind. Terraform creates them: apply one
            of the scenarios under <code>examples/terraform/scenarios/</code>.
          </div>
        </div>
      ) : (
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Resource group</th>
              <th>Location</th>
            </tr>
          </thead>
          <tbody>
            {matches.map((r) => {
              const cat = getCategoryForType(r.type);
              return (
                <tr
                  key={r.id}
                  className={styles.row}
                  onClick={() => navigate(`/explorer?q=${encodeURIComponent(r.name)}`)}
                >
                  <td className={styles.nameCell}>
                    {cat && <CategoryBadge code={cat.code} size={18} />}
                    {r.name}
                  </td>
                  <td className={styles.dim}>{r.type}</td>
                  <td className={styles.dim}>{resolveResourceGroup(r, resourceList) ?? '--'}</td>
                  <td className={styles.dim}>{r.location || '--'}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </PageShell>
  );
}

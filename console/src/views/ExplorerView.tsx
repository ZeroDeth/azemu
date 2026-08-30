import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { SideNav } from '../components/SideNav';
import { ResourceTree } from '../components/ResourceTree';
import { DetailBlade } from '../components/DetailBlade';
import { DockedLog } from '../components/DockedLog';
import { useResources } from '../hooks/useResources';
import { useRequestLog } from '../hooks/useRequestLog';
import { resolveResourceGroup } from '../types/resource';
import type { Resource } from '../types/resource';
import styles from './ExplorerView.module.css';

export function ExplorerView() {
  const { resourceList } = useResources();
  const { entries: logEntries } = useRequestLog();
  const [selected, setSelected] = useState<Resource | null>(null);
  const [params] = useSearchParams();
  const query = params.get('q')?.trim().toLowerCase() ?? '';

  // The search box in the top bar routes here with ?q=. Match on the three
  // things a user actually remembers: name, ARM type, and resource group.
  const visible = useMemo(() => {
    if (!query) return resourceList;
    return resourceList.filter((r) =>
      r.name.toLowerCase().includes(query) ||
      r.type.toLowerCase().includes(query) ||
      (resolveResourceGroup(r, resourceList) ?? '').toLowerCase().includes(query),
    );
  }, [resourceList, query]);

  return (
    <div className={styles.layout}>
      <div className={styles.middle}>
        <SideNav active="all-resources" compact width={188} />
        <ResourceTree
          resources={visible}
          selectedId={selected?.id}
          onSelect={setSelected}
        />
        {selected ? (
          <DetailBlade resource={selected} />
        ) : (
          <div className={styles.placeholder}>
            {query
              ? `${visible.length} of ${resourceList.length} resources match "${query}"`
              : 'Select a resource from the tree'}
          </div>
        )}
      </div>
      <DockedLog entries={logEntries} />
    </div>
  );
}

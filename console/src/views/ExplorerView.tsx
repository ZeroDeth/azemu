import { useState } from 'react';
import { SideNav } from '../components/SideNav';
import { ResourceTree } from '../components/ResourceTree';
import { DetailBlade } from '../components/DetailBlade';
import { DockedLog } from '../components/DockedLog';
import { useResources } from '../hooks/useResources';
import { useRequestLog } from '../hooks/useRequestLog';
import type { Resource } from '../types/resource';
import styles from './ExplorerView.module.css';

export function ExplorerView() {
  const { resourceList } = useResources();
  const { entries: logEntries } = useRequestLog();
  const [selected, setSelected] = useState<Resource | null>(null);

  return (
    <div className={styles.layout}>
      <div className={styles.middle}>
        <SideNav active="all-resources" compact width={188} />
        <ResourceTree
          resources={resourceList}
          selectedId={selected?.id}
          onSelect={setSelected}
        />
        {selected ? (
          <DetailBlade resource={selected} />
        ) : (
          <div className={styles.placeholder}>
            Select a resource from the tree
          </div>
        )}
      </div>
      <DockedLog entries={logEntries} />
    </div>
  );
}

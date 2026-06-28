import type { Resource, CategoryCode } from '../types/resource';
import { TYPE_TO_CATEGORY } from '../types/resource';
import { CategoryBadge } from './CategoryBadge';
import { Download, Upload } from 'lucide-react';
import styles from './InventoryTiles.module.css';

interface Props {
  resources: Resource[];
  onExport?: () => void;
  onImport?: () => void;
  onReset?: () => void;
}

interface Tile {
  code: CategoryCode;
  name: string;
  count: number;
}

function computeTiles(resources: Resource[]): Tile[] {
  const counts = new Map<string, number>();
  for (const r of resources) {
    counts.set(r.type, (counts.get(r.type) ?? 0) + 1);
  }

  const tiles: Tile[] = [];
  let otherCount = 0;
  for (const [type, count] of counts) {
    const entry = TYPE_TO_CATEGORY[type];
    if (entry) {
      tiles.push({ code: entry.code, name: entry.label, count });
    } else {
      otherCount += count;
    }
  }
  if (otherCount > 0) {
    tiles.push({ code: 'OT', name: 'Other', count: otherCount });
  }
  return tiles;
}

export function InventoryTiles({ resources, onExport, onImport, onReset }: Props) {
  const tiles = computeTiles(resources);

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.title}>Resource inventory</div>
        <div className={styles.actions}>
          <button className={styles.chip} onClick={onExport}>
            <Download size={11} strokeWidth={1.7} />
            Export
          </button>
          <button className={styles.chip} onClick={onImport}>
            <Upload size={11} strokeWidth={1.7} />
            Import
          </button>
          <button className={styles.resetChip} onClick={onReset}>
            Reset
          </button>
        </div>
      </div>
      <div className={styles.grid}>
        {tiles.length === 0 ? (
          <div className={styles.empty}>
            No resources yet — run <code>terraform apply</code> against azemu
          </div>
        ) : (
          tiles.map((t) => (
            <div key={t.code} className={styles.tile}>
              <CategoryBadge code={t.code} size={34} />
              <div className={styles.tileContent}>
                <div className={styles.count}>{t.count}</div>
                <div className={styles.typeName}>{t.name}</div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

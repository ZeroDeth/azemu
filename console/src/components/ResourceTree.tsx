import { useState, useMemo } from 'react';
import { ChevronRight, Plus } from 'lucide-react';
import { CategoryBadge } from './CategoryBadge';
import type { Resource, CategoryCode } from '../types/resource';
import { getResourceGroup, getCategoryForType } from '../types/resource';
import styles from './ResourceTree.module.css';

interface ResourceGroup {
  name: string;
  resources: Resource[];
}

interface Props {
  resources: Resource[];
  selectedId?: string | null;
  onSelect?: (resource: Resource) => void;
}

export function ResourceTree({ resources, selectedId, onSelect }: Props) {
  const groups = useMemo(() => {
    const map = new Map<string, Resource[]>();
    for (const r of resources) {
      if (r.type === 'Microsoft.Resources/resourceGroups') continue;
      const rg = getResourceGroup(r.id) ?? 'unknown';
      if (!map.has(rg)) map.set(rg, []);
      map.get(rg)!.push(r);
    }
    const result: ResourceGroup[] = [];
    for (const [name, res] of map) {
      result.push({ name, resources: res });
    }
    return result;
  }, [resources]);

  const [expanded, setExpanded] = useState<Set<string>>(() =>
    new Set(groups.map((g) => g.name)),
  );

  const toggle = (name: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  return (
    <div className={styles.tree}>
      <div className={styles.header}>
        <span className={styles.headerTitle}>Resource explorer</span>
        <Plus size={13} strokeWidth={1.7} color="#6e7681" />
      </div>
      <div className={styles.body}>
        {groups.map((group) => (
          <div key={group.name}>
            <button className={styles.rgRow} onClick={() => toggle(group.name)}>
              <ChevronRight
                size={11}
                strokeWidth={2}
                color="#6e7681"
                style={{
                  transform: expanded.has(group.name) ? 'rotate(90deg)' : 'none',
                  transition: 'transform 0.15s',
                }}
              />
              <CategoryBadge code="RG" size={18} />
              <span className={styles.rgName}>{group.name}</span>
              {!expanded.has(group.name) && (
                <span className={styles.rgCount}>{group.resources.length}</span>
              )}
            </button>
            {expanded.has(group.name) &&
              group.resources.map((r) => {
                const cat = getCategoryForType(r.type);
                const isSelected = r.id === selectedId;
                const selColor = cat?.color ?? '#8b949e';
                return (
                  <button
                    key={r.id}
                    className={styles.childRow}
                    onClick={() => onSelect?.(r)}
                    style={{
                      color: isSelected ? 'var(--text-primary)' : undefined,
                      borderLeftColor: isSelected ? selColor : 'transparent',
                      background: isSelected
                        ? hexToRgba(selColor, 0.08)
                        : undefined,
                    }}
                  >
                    {cat && <CategoryBadge code={cat.code as CategoryCode} size={18} />}
                    <span>{r.name}</span>
                  </button>
                );
              })}
          </div>
        ))}
      </div>
    </div>
  );
}

function hexToRgba(hex: string, alpha: number): string {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${alpha})`;
}

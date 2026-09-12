import { useMemo, useState } from 'react';
import { PageShell } from './PageShell';
import { RequestLog } from '../components/RequestLog';
import { StatusDot } from '../components/StatusDot';
import { useRequestLog } from '../hooks/useRequestLog';
import styles from './RequestLogView.module.css';

const FILTERS = ['All', 'Errors', 'Writes'] as const;
/** Methods that cannot change a resource, so they are not "writes". */
const READ_ONLY = new Set(['GET', 'HEAD', 'OPTIONS']);
type Filter = (typeof FILTERS)[number];

export function RequestLogView() {
  const { entries, connected } = useRequestLog();
  const [filter, setFilter] = useState<Filter>('All');
  const [needle, setNeedle] = useState('');

  const visible = useMemo(() => {
    const q = needle.trim().toLowerCase();
    return entries.filter((e) => {
      if (filter === 'Errors' && e.status < 400) return false;
      if (filter === 'Writes' && READ_ONLY.has(e.method)) return false;
      if (q && !e.path.toLowerCase().includes(q) && !e.method.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
  }, [entries, filter, needle]);

  const errors = entries.filter((e) => e.status >= 400).length;

  return (
    <PageShell
      active="request-log"
      title="Request log"
      subtitle="Every ARM call the provider makes, streamed live over SSE"
      aside={
        <>
          <StatusDot color={connected ? '#3fb950' : '#d29922'} glow />
          {entries.length} requests · {errors} errors
        </>
      }
    >
      <div className={styles.controls}>
        <div className={styles.filters}>
          {FILTERS.map((f) => (
            <button
              key={f}
              className={`${styles.filter} ${filter === f ? styles.filterActive : ''}`}
              onClick={() => setFilter(f)}
            >
              {f}
            </button>
          ))}
        </div>
        <input
          className={styles.search}
          value={needle}
          onChange={(e) => setNeedle(e.target.value)}
          placeholder="Filter by path or method..."
          aria-label="Filter requests"
        />
      </div>

      {visible.length === 0 ? (
        <div className={styles.empty}>
          {entries.length === 0
            ? 'No requests yet. Run terraform apply against azemu and they will appear here.'
            : `No requests match. ${entries.length} are hidden by the current filter.`}
        </div>
      ) : (
        <RequestLog entries={visible} />
      )}
    </PageShell>
  );
}

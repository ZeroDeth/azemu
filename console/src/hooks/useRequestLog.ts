import { useEffect, useRef, useState } from 'react';
import type { RequestLogEntry } from '../types/resource';

const MAX_ENTRIES = 200;

export function useRequestLog() {
  const [entries, setEntries] = useState<RequestLogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const es = new EventSource('/api/requests/stream');
    esRef.current = es;

    es.onopen = () => setConnected(true);

    es.onmessage = (event) => {
      const entry: RequestLogEntry = JSON.parse(event.data);
      setEntries((prev) => {
        const next = [...prev, entry];
        return next.length > MAX_ENTRIES ? next.slice(-MAX_ENTRIES) : next;
      });
    };

    es.onerror = () => {
      setConnected(false);
    };

    return () => {
      es.close();
      esRef.current = null;
    };
  }, []);

  return { entries, connected };
}

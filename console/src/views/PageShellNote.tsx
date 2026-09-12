import type { ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';
import styles from './PageShellNote.module.css';

interface Props {
  tone: 'warn' | 'info';
  children: ReactNode;
}

/** Inline callout for a fact the user needs before they act on the page. */
export function PageShellNote({ tone, children }: Props) {
  return (
    <div className={`${styles.note} ${tone === 'warn' ? styles.warn : styles.info}`}>
      <AlertTriangle size={14} strokeWidth={1.7} className={styles.icon} />
      <div>{children}</div>
    </div>
  );
}

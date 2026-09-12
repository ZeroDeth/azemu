import type { ReactNode } from 'react';
import { SideNav } from '../components/SideNav';
import styles from './PageShell.module.css';

interface Props {
  /** Nav item id to highlight. */
  active: string;
  title: string;
  subtitle?: string;
  /** Rendered on the right of the header row, for counts or controls. */
  aside?: ReactNode;
  children: ReactNode;
}

/**
 * Shared frame for the single-purpose views: side nav, a titled header, and a
 * scrolling body. The overview and explorer keep their own bespoke layouts.
 */
export function PageShell({ active, title, subtitle, aside, children }: Props) {
  return (
    <div className={styles.layout}>
      <SideNav active={active} />
      <div className={styles.main}>
        <div className={styles.header}>
          <div>
            <h1 className={styles.title}>{title}</h1>
            {subtitle && <div className={styles.subtitle}>{subtitle}</div>}
          </div>
          {aside && <div className={styles.aside}>{aside}</div>}
        </div>
        <div className={styles.body}>{children}</div>
      </div>
    </div>
  );
}

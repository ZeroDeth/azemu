import type { CategoryCode } from '../types/resource';
import { CATEGORY_COLORS } from '../types/resource';

interface Props {
  code: CategoryCode;
  size?: number;
}

export function CategoryBadge({ code, size = 22 }: Props) {
  const color = CATEGORY_COLORS[code];
  const r = parseInt(color.slice(1, 3), 16);
  const g = parseInt(color.slice(3, 5), 16);
  const b = parseInt(color.slice(5, 7), 16);

  return (
    <span
      style={{
        width: size,
        height: size,
        borderRadius: size >= 30 ? 8 : 5,
        background: `rgba(${r},${g},${b},0.16)`,
        color,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontFamily: 'var(--font-mono)',
        fontSize: size >= 30 ? 12 : 9.5,
        fontWeight: 700,
        flexShrink: 0,
      }}
    >
      {code}
    </span>
  );
}

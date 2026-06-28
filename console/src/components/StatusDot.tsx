interface Props {
  color?: string;
  size?: number;
  glow?: boolean;
}

export function StatusDot({ color = '#3fb950', size = 7, glow = false }: Props) {
  return (
    <span
      style={{
        width: size,
        height: size,
        borderRadius: '50%',
        background: color,
        boxShadow: glow ? `0 0 8px ${color}` : undefined,
        flexShrink: 0,
      }}
    />
  );
}

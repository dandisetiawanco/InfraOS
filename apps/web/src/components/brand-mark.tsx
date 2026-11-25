export function BrandMark({ size = 28 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      aria-hidden="true"
      className="shrink-0"
    >
      <rect x="4" y="4" width="56" height="56" rx="14" fill="#172A32" />
      <rect
        x="4.75"
        y="4.75"
        width="54.5"
        height="54.5"
        rx="13.25"
        stroke="#9DE1DD"
        strokeOpacity="0.32"
        strokeWidth="1.5"
      />
      <path d="M16 24.5L32 16L48 24.5L32 33L16 24.5Z" fill="#9DE1DD" />
      <path
        d="M16 33.5L32 42L48 33.5"
        stroke="#EDF4F6"
        strokeWidth="3.2"
        strokeLinecap="square"
      />
      <path
        d="M16 41.5L32 50L48 41.5"
        stroke="#9DE1DD"
        strokeOpacity="0.65"
        strokeWidth="3.2"
        strokeLinecap="square"
      />
      <circle cx="32" cy="24.5" r="3" fill="#172A32" />
    </svg>
  );
}

interface CoverThumbProps {
  title: string
  coverUrl?: string
  size?: 'sm' | 'lg'
}

const PALETTE = [
  'from-[#4a3f6b] to-[#b98fd6]',
  'from-[#17324a] to-[#6fc2c9]',
  'from-[#402326] to-[#c97b6e]',
  'from-[#2f3d2a] to-[#a3c17a]',
]

function paletteFor(title: string): string {
  let hash = 0
  for (let i = 0; i < title.length; i++) hash = (hash * 31 + title.charCodeAt(i)) >>> 0
  return PALETTE[hash % PALETTE.length]
}

export default function CoverThumb({ title, coverUrl, size = 'sm' }: CoverThumbProps) {
  const dims = size === 'sm' ? 'w-10 h-[58px]' : 'w-16 h-[94px]'
  if (coverUrl) {
    return (
      <img
        src={coverUrl}
        alt={`Cover of ${title}`}
        className={`${dims} object-cover rounded shadow-sm shrink-0`}
      />
    )
  }
  return (
    <div
      className={`${dims} rounded shadow-sm shrink-0 bg-gradient-to-br ${paletteFor(title)} flex items-end`}
      aria-hidden="true"
    >
      <span className="font-serif font-bold text-white/90 px-1 pb-1 text-lg leading-none">
        {title.charAt(0).toUpperCase()}
      </span>
    </div>
  )
}

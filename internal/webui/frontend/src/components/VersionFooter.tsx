import { versionLink } from '../version'

export default function VersionFooter({ version }: { version?: string }) {
  if (!version) return null
  const { label, href } = versionLink(version)
  return (
    <footer className="mt-12 pt-4 border-t border-line text-center font-mono text-xs text-muted">
      <a href={href} target="_blank" rel="noreferrer" className="hover:text-burgundy">
        audible-hardcover-sync {label}
      </a>
    </footer>
  )
}

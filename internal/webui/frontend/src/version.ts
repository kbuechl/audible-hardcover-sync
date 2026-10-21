const REPO = 'https://github.com/kbuechl/audible-hardcover-sync'

// Release builds link to their release notes; local builds report "dev".
export function versionLink(version: string): { label: string; href: string } {
  if (/^\d+\.\d+\.\d+/.test(version)) {
    return { label: `v${version}`, href: `${REPO}/releases/tag/v${version}` }
  }
  return { label: version, href: REPO }
}

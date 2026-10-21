import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import VersionFooter from './VersionFooter'
import { versionLink } from '../version'

describe('VersionFooter', () => {
  it('links a release version to its release notes', () => {
    render(<VersionFooter version="0.2.1" />)
    expect(screen.getByRole('link', { name: /v0\.2\.1/ })).toHaveAttribute(
      'href',
      'https://github.com/kbuechl/audible-hardcover-sync/releases/tag/v0.2.1',
    )
  })

  it('links a dev build to the repo', () => {
    expect(versionLink('dev')).toEqual({ label: 'dev', href: 'https://github.com/kbuechl/audible-hardcover-sync' })
  })

  it('renders nothing before status loads', () => {
    const { container } = render(<VersionFooter />)
    expect(container).toBeEmptyDOMElement()
  })
})

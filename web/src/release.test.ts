import { afterEach, describe, expect, it, vi } from 'vitest'
import { RELEASE, updateNotice } from './release'

describe('updateNotice', () => {
  afterEach(() => vi.unstubAllGlobals())
  it('stays quiet for a recent release and reminds after three months', () => {
    expect(updateNotice(new Date(RELEASE.date + 'T12:00:00Z'))).toBe('')
    expect(updateNotice(new Date('2027-03-01T00:00:00Z'))).toMatch(/never checks for updates by itself/)
  })
  it('never makes a network request', () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    updateNotice(new Date('2028-01-01T00:00:00Z'))
    expect(fetch).not.toHaveBeenCalled()
  })
})

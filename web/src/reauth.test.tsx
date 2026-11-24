import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ReauthDialog } from './reauth'
import { download, setReauthHandler } from './api'

const reply = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  setReauthHandler(null)
})

describe('ReauthDialog', () => {
  const submit = async (status: number, body: unknown) => {
    vi.stubGlobal('fetch', vi.fn(async () => reply(status, body)))
    render(<ReauthDialog onDone={() => {}} />)
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'synthetic password' } })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    return (await screen.findByRole('alert')).textContent
  }

  it('says the password did not match only for a wrong password', async () => {
    expect(await submit(401, { error: 'That password did not match.' })).toBe('That password did not match. Try again.')
  })

  it('reports too many attempts as such', async () => {
    expect(await submit(429, { error: 'Too many sign-in attempts. Try again in ten minutes.' })).toBe(
      'Too many sign-in attempts. Try again in ten minutes.',
    )
  })
})

describe('download', () => {
  it('asks for the password before saving a file that needs it', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        calls.push(url)
        if (url === '/api/reauth') return reply(200, { ok: true })
        return calls.filter((u) => u === '/api/encounters/x/export').length === 1
          ? reply(403, { error: 'Enter your password again to continue.', reauth: true })
          : reply(200, { revision: 3 })
      }),
    )
    const asked = vi.fn(async () => true)
    setReauthHandler(asked)
    const saved: { name: string; href: string }[] = []
    URL.createObjectURL = vi.fn(() => 'blob:synthetic')
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      saved.push({ name: this.download, href: this.href })
    })
    await download('/encounters/x/export', 'healthctl.json')
    await waitFor(() => expect(saved).toEqual([{ name: 'healthctl.json', href: 'blob:synthetic' }]))
    expect(asked).toHaveBeenCalledOnce()
    expect(calls.filter((u) => u === '/api/encounters/x/export')).toHaveLength(2)
  })
})

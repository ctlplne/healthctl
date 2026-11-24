import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { Auth } from './forms'
import { ReauthDialog } from './reauth'
import type { Session } from './types'

afterEach(cleanup)

const session = (setupRequired: boolean): Session => ({
  assessmentNotice: { version: 'v', title: 'Notice', body: [] } as unknown as Session['assessmentNotice'],
  setupRequired,
  authenticated: false,
  csrf: '',
  displayName: 'healthctl',
  notice: 'Synthetic notice.',
  noticeVersion: 'v',
  emergency: '',
})

const auth = (setupRequired: boolean) =>
  render(<Auth session={session(setupRequired)} run={() => {}} busy={false} onLogin={() => {}} error="" />)

describe('Forgot your password?', () => {
  it('shows every way to reset on the sign-in page', () => {
    auth(false)
    expect(screen.getByText('Forgot your password?')).toBeTruthy()
    expect(screen.getByText(/open the healthctl launcher and choose/)).toBeTruthy()
    expect(screen.getByText('python3 scripts/maintenance.py recover-owner')).toBeTruthy()
    expect(screen.getByText(/signs out every device, ends shared brief links and disconnects/)).toBeTruthy()
  })

  it('is not offered while creating the owner account', () => {
    auth(true)
    expect(screen.queryByText('Forgot your password?')).toBeNull()
  })

  it('is offered when the app asks for the password again', () => {
    render(<ReauthDialog onDone={() => {}} />)
    expect(screen.getByText('Forgot your password?')).toBeTruthy()
  })
})

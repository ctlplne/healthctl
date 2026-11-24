import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { StabilityNote } from './report'
import type { Assessment } from './types'

afterEach(cleanup)

const items = (unstable: boolean[]) => unstable.map((u) => ({ unstable: u })) as unknown as Assessment['items']

describe('StabilityNote', () => {
  it('says when the second run failed, so no comparison was made', () => {
    render(<StabilityNote a={{ items: items([false]), stabilityFailed: true }} />)
    expect(screen.getByRole('note').textContent).toMatch(/second run for the stability check failed/)
  })

  it('says when every possibility appeared in both runs', () => {
    render(<StabilityNote a={{ items: items([false, false]), stabilityChecked: true }} />)
    expect(screen.getByRole('note').textContent).toMatch(/every possibility appeared in both runs/)
  })

  it('counts the possibilities that changed', () => {
    render(<StabilityNote a={{ items: items([true, false, true]), stabilityChecked: true }} />)
    expect(screen.getByRole('note').textContent).toMatch(/2 possibilities changed between the runs and are marked uncertain/)
  })

  it('shows nothing when no stability check was requested', () => {
    const { container } = render(<StabilityNote a={{ items: items([false]) }} />)
    expect(container.innerHTML).toBe('')
  })
})

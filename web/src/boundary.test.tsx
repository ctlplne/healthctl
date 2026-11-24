import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SafetyBoundary } from './boundary'
import type { Warning } from './types'

function Broken(): never {
  throw new Error('render failure')
}

const warning = { id: 'w1', active: true, message: 'Seek local emergency care immediately.' } as Warning

describe('SafetyBoundary', () => {
  it('keeps active urgent warnings visible when the page fails', () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <SafetyBoundary warnings={[warning, { ...warning, id: 'w2', active: false, message: 'Old warning' }]} resetKey="record:e1">
        <Broken />
      </SafetyBoundary>,
    )
    quiet.mockRestore()
    expect(screen.getByRole('alert').textContent).toContain('Seek local emergency care immediately.')
    expect(screen.queryByText('Old warning')).toBeNull()
    expect(screen.getByText('This page could not be shown')).toBeTruthy()
  })
})

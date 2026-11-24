import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SafetyNet, safetyCheckDue } from './safetynet'
import type { Encounter } from './types'

describe('SafetyNet', () => {
  it('is due until answered for the current history revision', () => {
    const e = { historyRevision: 3, safetyChecks: [{ historyRevision: 2, point: 'review', answer: 'none', at: '' }] } as unknown as Encounter
    expect(safetyCheckDue(e, 'review')).toBe(true)
    e.safetyChecks!.push({ historyRevision: 3, point: 'review', answer: 'none', at: '' })
    expect(safetyCheckDue(e, 'review')).toBe(false)
    expect(safetyCheckDue(e, 'return')).toBe(true)
  })
  it('sends the chosen answer', () => {
    const onAnswer = vi.fn()
    render(<SafetyNet point="review" busy={false} onAnswer={onAnswer} />)
    fireEvent.click(screen.getByText('I may be having an emergency now'))
    expect(onAnswer).toHaveBeenCalledWith('emergency')
  })
})

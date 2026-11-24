import { describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useQuestionDraft } from './draft'

describe('useQuestionDraft', () => {
  it('sets a draft aside when the question changes underneath it', () => {
    const reset = vi.fn()
    const { result, rerender } = renderHook(({ q }) => useQuestionDraft(q, reset), { initialProps: { q: 'When did it start?' } })
    act(() => result.current.setAnswer('Yesterday morning'))
    rerender({ q: 'Is it getting worse?' })
    expect(result.current.answer).toBe('')
    expect(result.current.setAside).toEqual({ text: 'Yesterday morning', question: 'When did it start?' })
    expect(reset).toHaveBeenCalledTimes(1)
    act(() => result.current.restore())
    expect(result.current.answer).toBe('Yesterday morning')
    expect(result.current.setAside).toBeNull()
  })
  it('leaves an empty draft alone', () => {
    const { result, rerender } = renderHook(({ q }) => useQuestionDraft(q), { initialProps: { q: 'First?' } })
    rerender({ q: 'Second?' })
    expect(result.current.setAside).toBeNull()
  })
})

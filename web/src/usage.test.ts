import { describe, expect, it } from 'vitest'
import { usageText } from './usage'

describe('usageText', () => {
  it('adds up calls, tokens, waiting and a complete cost', () => {
    expect(
      usageText({ calls: 3, failedCalls: 1, inputTokens: 1500, outputTokens: 500, waitSeconds: 12.4, estimatedUSD: 0.024, costComplete: true }),
    ).toBe('3 model calls (1 failed) · 2,000 tokens · 12 s waiting · about $0.024 (estimate)')
  })
  it('never shows a partial cost as the total', () => {
    expect(usageText({ calls: 1, failedCalls: 0, inputTokens: 10, outputTokens: 0, waitSeconds: 1, costComplete: false })).toContain('cost unknown')
  })
  it('says when nothing was sent', () => {
    expect(usageText({ calls: 0, failedCalls: 0, inputTokens: 0, outputTokens: 0, waitSeconds: 0, costComplete: false })).toBe('No model calls yet.')
  })
})

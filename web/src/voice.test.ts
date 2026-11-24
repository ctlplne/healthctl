import { describe, expect, it } from 'vitest'
import { resample } from './voice'

describe('resample', () => {
  it('keeps 16 kHz audio as it is', () => {
    const a = new Float32Array([0.1, 0.2, 0.3])
    expect(resample(a, 16000)).toBe(a)
  })
  it('averages 48 kHz audio down to 16 kHz', () => {
    const out = resample(new Float32Array([0, 0.3, 0.6, 0.9, 0.9, 0.9]), 48000)
    expect(out.length).toBe(2)
    expect(out[0]).toBeCloseTo(0.3)
    expect(out[1]).toBeCloseTo(0.9)
  })
  it('handles 44.1 kHz without dropping or inventing time', () => {
    const second = new Float32Array(44100).fill(0.5)
    const out = resample(second, 44100)
    expect(out.length).toBe(16000)
    expect(Math.max(...out)).toBeCloseTo(0.5)
  })
})

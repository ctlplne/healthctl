import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { CompactList } from './handoff'

describe('CompactList', () => {
  it('says how many items a shortened list leaves out', () => {
    const { container } = render(<CompactList values={['a', 'b', 'c', 'd', 'e', 'f', 'g']} limit={5} />)
    expect(container.querySelectorAll('li')).toHaveLength(5)
    expect(container.textContent).toContain('2 more in the provenance appendix.')
  })
  it('shows every item when there is room', () => {
    const { container } = render(<CompactList values={['Conflict · Fever']} />)
    expect(container.querySelectorAll('li')).toHaveLength(1)
    expect(container.textContent).not.toContain('more')
  })
})

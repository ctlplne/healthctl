import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { Gaps, RedFlags } from './redflags'

describe('RedFlags', () => {
  it('names each question group and reports the answer for its key', () => {
    const onAnswer = vi.fn()
    render(
      <RedFlags
        busy={false}
        onAnswer={onAnswer}
        due={[{ key: 'red_flag:respiratory:blood', pathway: 'respiratory', question: 'Have you coughed up any blood?' }]}
      />,
    )
    const group = screen.getByRole('group', { name: 'Have you coughed up any blood?' })
    fireEvent.click(group.querySelector('button')!)
    expect(onAnswer).toHaveBeenCalledWith('red_flag:respiratory:blood', 'yes')
  })

  it('shows nothing when no safety question is due', () => {
    const { container } = render(<RedFlags busy={false} onAnswer={() => {}} due={[]} />)
    expect(container.innerHTML).toBe('')
  })

  it('lists every gap in the app wording', () => {
    render(<Gaps gaps={[{ pathway: 'headache', element: 'onset', note: 'Not yet asked: when it started.' }]} />)
    expect(screen.getByText('Not yet asked: when it started.')).toBeTruthy()
  })
})

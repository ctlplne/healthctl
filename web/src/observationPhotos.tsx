import { useState } from 'react'
import type { Encounter, Fields } from './types'
import type { Run } from './forms'
import { api } from './api'

type Props = {
  e: Encounter
  busy: boolean
  run: Run
  refresh: (next: Encounter) => void
  act: (cmd: Fields) => Promise<boolean>
}
export function ObservationPhotos({ e, busy, run, refresh, act }: Props) {
  const [area, setArea] = useState(
    e.pathways?.some((p) => p.id === 'eye_appearance')
      ? 'Eye'
      : e.pathways?.some((p) => p.id === 'nail_change')
        ? 'Fingernails or hands'
        : 'Other visible change',
  )
  const [words, setWords] = useState('')
  const [file, setFile] = useState<File>()
  const [requestId, setRequestId] = useState(() => crypto.randomUUID())
  const [reset, setReset] = useState(0)
  const [remove, setRemove] = useState<{ id: string; requestId: string }>()
  const photos = e.photos || []
  const blocked =
    !!e.pending || !!e.scopeReason || e.warnings.some((w) => w.active)
  function choose(next?: File) {
    setFile(next)
    setRequestId(crypto.randomUUID())
  }
  return (
    <details className="panel visual-record">
      <summary>
        Optional photo of a visible change
        {photos.length > 0 ? ' · ' + photos.length + ' saved' : ''}
      </summary>
      <p>
        Describe what you see in your interview answer. You can also keep a
        photo on this installation to show your clinician. The model receives
        your words only.
      </p>
      {photos.map((photo) => (
        <p key={photo.id}>
          <a
            href={'/api/encounters/' + e.id + '/photos/' + photo.id}
            target="_blank"
            rel="noreferrer"
          >
            View saved photo · {photo.area} ·{' '}
            {new Date(photo.createdAt).toLocaleDateString()}
          </a>{' '}
          {remove?.id === photo.id ? (
            <>
              <button
                type="button"
                className="secondary"
                disabled={busy}
                onClick={() =>
                  run(async () => {
                    const next = await api<Encounter>(
                      '/encounters/' + e.id + '/photos/' + photo.id,
                      'DELETE',
                      { revision: e.revision, requestId: remove.requestId },
                    )
                    refresh(next)
                    setRemove(undefined)
                  })
                }
              >
                Confirm removal
              </button>{' '}
              <button
                type="button"
                className="text-button"
                onClick={() => setRemove(undefined)}
              >
                Keep photo
              </button>
            </>
          ) : (
            <button
              type="button"
              className="text-button"
              disabled={busy || !!e.pending}
              onClick={() =>
                setRemove({ id: photo.id, requestId: crypto.randomUUID() })
              }
            >
              Remove photo
            </button>
          )}
        </p>
      ))}
      {!blocked && (
        <label>
          What does your photo show?
          <select
            value={area}
            onChange={(ev) => {
              setArea(ev.target.value)
              setRequestId(crypto.randomUUID())
            }}
          >
            {[
              'Eye',
              'Fingernails or hands',
              'Toenails or feet',
              'Other visible change',
            ].map((a) => (
              <option key={a}>{a}</option>
            ))}
          </select>
        </label>
      )}
      {!blocked && photos.length < 12 && (
        <form
          onSubmit={(ev) => {
            ev.preventDefault()
            if (!file || busy) return
            const data = new FormData()
            data.set('revision', String(e.revision))
            data.set('requestId', requestId)
            data.set('photoArea', area)
            data.set('belongsTo', 'self')
            data.set('file', file)
            run(async () => {
              const updated = await api<Encounter>(
                '/encounters/' + e.id + '/documents',
                'POST',
                data,
              )
              choose(undefined)
              setReset((n) => n + 1)
              refresh(updated)
            })
          }}
        >
          <p>
            Use ordinary lighting and include just the area you want to show. A
            photo cannot establish the cause. Seek urgent medical advice today
            if your skin or the whites of your eyes look newly yellow.
          </p>
          <div key={reset}>
            <label>
              Take a photo with your camera
              <input
                type="file"
                accept="image/jpeg,image/png"
                capture="environment"
                onChange={(ev) => choose(ev.target.files?.[0])}
              />
            </label>
            <label>
              Or choose a saved photo
              <input
                type="file"
                accept="image/jpeg,image/png"
                onChange={(ev) => choose(ev.target.files?.[0])}
              />
            </label>
          </div>
          <small>
            JPEG or PNG, up to 12 MiB. A resized copy is saved without embedded
            camera metadata.
          </small>
          {file && <p>Selected · {file.name}</p>}
          <button type="submit" className="secondary" disabled={busy || !file}>
            Save photo locally
          </button>
        </form>
      )}
      {!blocked && !e.demo && (
        <form
          onSubmit={(ev) => {
            ev.preventDefault()
            if (words.trim() && !busy)
              void act({
                action: 'visual_observation',
                category: area,
                text: words.trim(),
              }).then((saved) => {
                if (saved) setWords('')
              })
          }}
        >
          <label>
            Describe the visible change in your own words
            <textarea
              rows={2}
              maxLength={2000}
              required
              value={words}
              onChange={(ev) => setWords(ev.target.value)}
            />
          </label>
          <small>
            These words join your encounter history for the selected model. You
            can add them without a photo.
          </small>
          <button type="submit" disabled={!words.trim() || busy}>
            Add observation to interview
          </button>
        </form>
      )}
    </details>
  )
}

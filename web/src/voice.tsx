import { useEffect, useRef, useState } from 'react'
import { ScreenAlert, type ScreenResult } from './screen'
import { api } from './api'

// resample converts captured audio to the 16 kHz the transcriber expects.
// Firefox records at the device rate and won't connect a 16 kHz context to
// the microphone, so the app averages down to 16 kHz itself.
export function resample(input: Float32Array, from: number, to = 16000): Float32Array {
  if (from === to) return input
  const ratio = from / to
  const out = new Float32Array(Math.floor(input.length / ratio))
  for (let i = 0; i < out.length; i++) {
    const start = Math.floor(i * ratio)
    const end = Math.min(input.length, Math.max(start + 1, Math.floor((i + 1) * ratio)))
    let sum = 0
    for (let j = start; j < end; j++) sum += input[j]
    out[i] = sum / (end - start)
  }
  return out
}

export function encodeWav(chunks: Float32Array[]) {
  const length = chunks.reduce((sum, chunk) => sum + chunk.length, 0)
  const bytes = new ArrayBuffer(44 + 2 * length)
  const view = new DataView(bytes)
  const label = (i: number, s: string) =>
    [...s].forEach((c, j) => view.setUint8(i + j, c.charCodeAt(0)))
  label(0, 'RIFF')
  view.setUint32(4, 36 + 2 * length, true)
  label(8, 'WAVEfmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, 16000, true)
  view.setUint32(28, 32000, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  label(36, 'data')
  view.setUint32(40, 2 * length, true)
  let i = 44
  for (const chunk of chunks)
    for (const sample of chunk) {
      view.setInt16(
        i,
        Math.round(
          Math.max(-1, Math.min(1, sample)) * (sample < 0 ? 32768 : 32767),
        ),
        true,
      )
      i += 2
    }
  return new Blob([bytes], { type: 'audio/wav' })
}

// Include the surrounding phrase so that a negation or unit can be corrected in context.
export function ReadBack({
  words,
  expanded = false,
}: {
  words: string
  expanded?: boolean
}) {
  const [voice, setVoice] = useState<SpeechSynthesisVoice>()
  useEffect(() => {
    if (!('speechSynthesis' in window)) return
    const synth = window.speechSynthesis
    const refresh = () =>
      setVoice(
        synth
          .getVoices()
          .find((v) => v.localService && /^en(?:-|$)/i.test(v.lang)),
      )
    refresh()
    synth.addEventListener('voiceschanged', refresh)
    return () => synth.removeEventListener('voiceschanged', refresh)
  }, [])
  const cues =
    /\b(?:left|right|both|no|not|never|without|didn['’]t|wasn['’]t|can['’]t|once|twice|one|two|three|five|ten|fifteen|fifty|medicine|medication|tablet|pills?|capsule|taking|took|dose|mg|mcg|ml|hours?|minutes?|\d+)\b/i
  const phrases = words
    .trim()
    .split(/(?<=[.!?])\s+|[;\n]+/)
    .map((s) => s.trim())
    .filter((v) => cues.test(v))
    .slice(0, 6)
  if (!phrases.length) return null
  return (
    <details open={expanded || undefined}>
      <summary>Check the exact wording</summary>
      {voice && (
        <button
          type="button"
          className="text-button"
          onClick={() => {
            const reading = new SpeechSynthesisUtterance(phrases.join(' '))
            reading.voice = voice
            window.speechSynthesis.cancel()
            window.speechSynthesis.speak(reading)
          }}
        >
          Read these words back on this device
        </button>
      )}
      {phrases.map((p, i) => (
        <p key={i}>“{p}”</p>
      ))}
      <p>
        Check the name, amount, timing, side and any negatives. Edit the words
        above if needed.
      </p>
    </details>
  )
}

export function VoiceInput({
  onUse,
  limit = 4000,
}: {
  onUse: (value: string) => void
  limit?: number
}) {
  const [open, setOpen] = useState(false),
    [caps, setCaps] = useState<{ local: boolean; openai: boolean }>(),
    [mode, setMode] = useState('local'),
    [recording, setRecording] = useState(false),
    [busy, setBusy] = useState(false),
    [clip, setClip] = useState<Blob>(),
    [draft, setDraft] = useState(''),
    [warning, setWarning] = useState<ScreenResult>(),
    [checked, setChecked] = useState(false),
    [error, setError] = useState('')
  const request = useRef<AbortController | null>(null)
  const stop = useRef<((keep: boolean) => void) | null>(null),
    alive = useRef(true)
  useEffect(() => {
    alive.current = true
    return () => {
      request.current?.abort()
      alive.current = false
      stop.current?.(false)
    }
  }, [])
  useEffect(() => {
    if (open)
      api<{ local: boolean; openai: boolean }>('/voice')
        .then(setCaps)
        .catch(() =>
          setError('Could not check speech services. You can still type.'),
        )
  }, [open])
  async function record() {
    if (busy || stop.current) return
    setBusy(true)
    setError('')
    let stream: MediaStream | undefined, ctx: AudioContext | undefined
    try {
      stream = await navigator.mediaDevices.getUserMedia({
        audio: { channelCount: 1 },
      })
      if (!alive.current) {
        stream.getTracks().forEach((t) => t.stop())
        return
      }
      ctx = new AudioContext({ sampleRate: 16000 })
      let src: MediaStreamAudioSourceNode
      try {
        src = ctx.createMediaStreamSource(stream)
      } catch {
        // Firefox: record at the device rate and resample each chunk.
        await ctx.close()
        ctx = new AudioContext()
        src = ctx.createMediaStreamSource(stream)
      }
      const rate = ctx.sampleRate
      await ctx.audioWorklet.addModule('/voice-worklet.js')
      if (!alive.current) {
        stream.getTracks().forEach((t) => t.stop())
        await ctx.close()
        return
      }
      const node = new AudioWorkletNode(ctx, 'healthctl-microphone')
      const mute = ctx.createGain()
      mute.gain.value = 0
      src.connect(node)
      node.connect(mute)
      mute.connect(ctx.destination)
      const chunks: Float32Array[] = []
      let samples = 0
      const timer = window.setTimeout(() => stop.current?.(true), 90000)
      stop.current = (keep) => {
        window.clearTimeout(timer)
        node.port.onmessage = null
        src.disconnect()
        node.disconnect()
        mute.disconnect()
        stream?.getTracks().forEach((t) => t.stop())
        void ctx?.close()
        stop.current = null
        if (alive.current) {
          setRecording(false)
          if (keep && samples) setClip(encodeWav(chunks))
        }
      }
      node.port.onmessage = ({ data }: { data: Float32Array }) => {
        const part = resample(data, rate).subarray(0, 1440000 - samples)
        chunks.push(part)
        samples += part.length
        if (samples >= 1440000) stop.current?.(true)
      }
      await ctx.resume()
      setClip(undefined)
      setDraft('')
      setChecked(false)
      setRecording(true)
    } catch {
      stop.current?.(false)
      stream?.getTracks().forEach((t) => t.stop())
      if (ctx && ctx.state !== 'closed') void ctx.close()
      setError(
        'The microphone could not start. Check microphone permission, type, or paste a transcript.',
      )
    } finally {
      if (alive.current) setBusy(false)
    }
  }
  async function transcribe() {
    if (!clip || request.current) return
    const controller = new AbortController()
    request.current = controller
    const timeout = window.setTimeout(() => controller.abort(), 130000)
    setBusy(true)
    setError('')
    try {
      const r = await api<{ draft: string; warning?: ScreenResult }>(
        '/voice?destination=' + mode,
        'POST',
        clip,
        {
          audioConsent: mode === 'openai',
          signal: controller.signal,
        },
      )
      if (!alive.current) return
      setDraft(r.draft)
      setWarning(r.warning)
      setChecked(false)
    } catch (err) {
      if (alive.current)
        setError(err instanceof Error ? err.message : 'Transcription failed.')
    } finally {
      window.clearTimeout(timeout)
      request.current = null
      if (alive.current) setBusy(false)
    }
  }
  return (
    <div className="voice-input">
      <button
        type="button"
        className="text-button"
        disabled={busy || recording}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        Tell me what happened aloud
      </button>
      {open && (
        <div className="side-panel">
          <label>
            Transcription method
            <select
              value={mode}
              disabled={busy || recording}
              onChange={(e) => {
                setMode(e.target.value)
                setChecked(false)
              }}
            >
              <option value="local">Offline on this installation</option>
              <option value="openai" disabled={!caps?.openai}>
                OpenAI API · separate billing
              </option>
              <option value="paste">Paste dictation from another app</option>
            </select>
          </label>
          {mode === 'paste' ? (
            <p>
              Dictate in the ChatGPT app or another tool you trust, review its
              transcript, and paste it below. Linked ChatGPT plans do not accept
              recordings from healthctl.
            </p>
          ) : (
            <>
              <p>
                {mode === 'local'
                  ? caps?.local === false
                    ? 'Whisper is unavailable on this installation. Choose another method.'
                    : 'Whisper transcribes on the machine running healthctl.'
                  : 'Sending this clip uses the configured OpenAI API account at api.openai.com.'}
              </p>
              <button
                type="button"
                className="secondary"
                disabled={
                  busy || (!recording && mode === 'local' && !caps?.local)
                }
                onClick={() =>
                  recording ? stop.current?.(true) : void record()
                }
              >
                {recording ? 'Stop recording' : 'Record up to 90 seconds'}
              </button>
              {clip && !recording && (
                <button
                  type="button"
                  className="secondary"
                  disabled={busy}
                  onClick={() => void transcribe()}
                >
                  {busy
                    ? 'Transcribing…'
                    : mode === 'local'
                      ? 'Transcribe locally'
                      : 'Send this recording to the OpenAI API'}
                </button>
              )}
            </>
          )}
          {(draft || mode === 'paste') && (
            <>
              {warning && <ScreenAlert warning={warning} />}
              <label>
                Transcript to review
                <textarea
 autoComplete="off"
                  value={draft}
                  onChange={(e) => {
                    setDraft(e.target.value)
                    setChecked(false)
                  }}
                />
              </label>
              <ReadBack words={draft} expanded />
              <label>
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={(e) => setChecked(e.target.checked)}
                />{' '}
                I checked names, numbers, left or right, and words like “no”.
              </label>
              <button
                type="button"
                className="secondary"
                disabled={
                  !checked ||
                  !draft.trim() ||
                  new TextEncoder().encode(draft.trim()).length > limit
                }
                onClick={() => {
                  onUse(draft.trim())
                  setClip(undefined)
                  setDraft('')
                  setOpen(false)
                  setChecked(false)
                }}
              >
                Use reviewed words
              </button>
            </>
          )}
          {error && <p role="alert">{error}</p>}
        </div>
      )}
    </div>
  )
}

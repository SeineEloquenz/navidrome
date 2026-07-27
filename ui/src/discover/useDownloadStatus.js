import { useEffect, useState } from 'react'
import { getStatus } from './dlClient'

// Single shared poller for /dl/status. Both the Discover queue panel and the
// menu indicator subscribe to it, so there's one interval and one source of
// truth. finished_items is process-global on SomeDL's side, so we baseline
// what's already finished at the first poll and only surface items that finish
// afterwards — regardless of whether we caught them active (short downloads can
// finish between polls). Labels come from the active phase when we saw it.
const POLL_MS = 2500
const MAX_FINISHED = 15

let state = { active: [], finished: [], queued: 0 }
let baseline = null
const seen = new Map()
const finishedItems = new Map()
const listeners = new Set()
let timer = null

const emit = () => listeners.forEach((l) => l(state))

const poll = async () => {
  try {
    const data = await getStatus()
    const active = data.active || []
    const finishedList = data.finished || []
    active.forEach((it) => seen.set(it.id, it.label))
    if (baseline === null) {
      baseline = new Set(finishedList.map((it) => it.id))
    }
    finishedList.forEach((it) => {
      if (!baseline.has(it.id) && !finishedItems.has(it.id)) {
        finishedItems.set(it.id, {
          id: it.id,
          label: it.label || seen.get(it.id) || '',
          status: it.status,
        })
      }
    })
    state = {
      active,
      queued: data.queued || 0,
      finished: Array.from(finishedItems.values()).slice(-MAX_FINISHED),
    }
    emit()
  } catch {
    // DL backend unavailable; keep last known state.
  }
}

export const useDownloadStatus = () => {
  const [snapshot, setSnapshot] = useState(state)
  useEffect(() => {
    listeners.add(setSnapshot)
    setSnapshot(state)
    if (!timer) {
      poll()
      timer = setInterval(poll, POLL_MS)
    }
    return () => {
      listeners.delete(setSnapshot)
      if (listeners.size === 0 && timer) {
        clearInterval(timer)
        timer = null
      }
    }
  }, [])
  return snapshot
}

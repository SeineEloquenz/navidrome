import { httpClient } from '../dataProvider'
import { REST_URL } from '../consts'

const base = `${REST_URL}/dl`

const post = (path, body) =>
  httpClient(`${base}${path}`, {
    method: 'POST',
    body: JSON.stringify(body),
    headers: new Headers({
      'Content-Type': 'application/json',
      Accept: 'application/json',
    }),
  }).then((r) => r.json)

export const search = (q, type) =>
  httpClient(`${base}/search?q=${encodeURIComponent(q)}&type=${type}`).then(
    (r) => r.json,
  )

export const getArtist = (id) =>
  httpClient(`${base}/artist/${encodeURIComponent(id)}`).then((r) => r.json)

export const getAlbum = (id) =>
  httpClient(`${base}/album/${encodeURIComponent(id)}`).then((r) => r.json)

export const downloadItems = (items) => post('/download', { items })

export const getStatus = () => httpClient(`${base}/status`).then((r) => r.json)

export const retryDownloads = (ids) => post('/retry', { ids })

export const importUrl = (url, name) => post('/import', { url, name })

export const completePreview = (body) => post('/complete/preview', body)

export const complete = (albumId, present) =>
  post('/complete', { albumId, present })

export const downloadAlbum = async (id) => {
  const { tracks } = await getAlbum(id)
  const items = (tracks || []).map((t) => ({ videoId: t.id }))
  return items.length ? downloadItems(items) : { enqueued: 0 }
}

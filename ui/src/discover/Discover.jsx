import React, { useState } from 'react'
import { Title, useTranslate, useNotify } from 'react-admin'
import {
  Box,
  Tab,
  Tabs,
  TextField,
  InputAdornment,
  IconButton,
  Button,
  CircularProgress,
  Typography,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import SearchIcon from '@material-ui/icons/Search'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import ResultCard from './ResultCard'
import DownloadQueue from './DownloadQueue'
import AlbumTracksDialog from './AlbumTracksDialog'
import {
  search as dlSearch,
  getArtist,
  downloadAlbum,
  downloadItems,
  importUrl,
} from './dlClient'

// The first three filter a search; "import" is a separate mode that takes a URL.
const MODES = [
  { key: 'album', label: 'Albums' },
  { key: 'track', label: 'Tracks' },
  { key: 'artist', label: 'Artists' },
  { key: 'import', label: 'Import' },
]

const useStyles = makeStyles((theme) => ({
  root: { padding: theme.spacing(2) },
  searchRow: {
    display: 'flex',
    gap: theme.spacing(2),
    alignItems: 'center',
    marginBottom: theme.spacing(2),
    flexWrap: 'wrap',
  },
  search: { flex: 1, minWidth: 240 },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))',
    gap: theme.spacing(2),
  },
  section: { margin: theme.spacing(2, 0, 1) },
  empty: {
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(4),
    textAlign: 'center',
  },
}))

const Discover = () => {
  const classes = useStyles()
  const translate = useTranslate()
  const notify = useNotify()

  const [query, setQuery] = useState('')
  const [type, setType] = useState('album')
  const [results, setResults] = useState([])
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const [artist, setArtist] = useState(null)
  const [importLink, setImportLink] = useState('')
  const [importName, setImportName] = useState('')
  const [importing, setImporting] = useState(false)
  // The card outlives albumOpen so the dialog title doesn't blank out mid-fade.
  const [albumCard, setAlbumCard] = useState(null)
  const [albumOpen, setAlbumOpen] = useState(false)

  const showAlbum = (card) => () => {
    setAlbumCard(card)
    setAlbumOpen(true)
  }

  const runSearch = async (q = query, t = type) => {
    if (!q.trim()) return
    setLoading(true)
    setArtist(null)
    try {
      setResults((await dlSearch(q, t)) || [])
      setSearched(true)
    } catch (e) {
      notify('Search failed: ' + e.message, 'warning')
    } finally {
      setLoading(false)
    }
  }

  const changeQuery = (v) => {
    setQuery(v)
    if (!v.trim()) {
      setResults([])
      setSearched(false)
    }
  }

  const changeType = (t) => {
    setType(t)
    if (t === 'import') {
      setArtist(null)
      return
    }
    if (query.trim()) runSearch(query, t)
  }

  const queued = (res) => notify(`Queued ${res?.enqueued ?? 0} download(s)`)

  const onImport = async () => {
    if (!importLink.trim() || importing) return
    setImporting(true)
    try {
      const res = await importUrl(importLink.trim(), importName.trim())
      notify(
        `Importing ${res.enqueued} track(s). Playlist "${res.playlist}" appears once downloads finish`,
      )
      setImportLink('')
      setImportName('')
    } catch (e) {
      notify('Import failed: ' + e.message, 'warning')
    } finally {
      setImporting(false)
    }
  }

  const onDownloadAlbum = (card) => async () => {
    try {
      queued(await downloadAlbum(card.id))
    } catch (e) {
      notify('Download failed: ' + e.message, 'warning')
    }
  }

  const onDownloadTrack = (card) => async () => {
    try {
      queued(await downloadItems([{ videoId: card.id }]))
    } catch (e) {
      notify('Download failed: ' + e.message, 'warning')
    }
  }

  const openArtist = (card) => async () => {
    setLoading(true)
    try {
      setArtist(await getArtist(card.id))
    } catch (e) {
      notify('Failed to load artist: ' + e.message, 'warning')
    } finally {
      setLoading(false)
    }
  }

  const renderCard = (card) => {
    if (card.kind === 'artist') {
      return <ResultCard key={card.id} card={card} onOpen={openArtist(card)} />
    }
    if (card.kind === 'track') {
      return (
        <ResultCard
          key={card.id}
          card={card}
          onDownload={onDownloadTrack(card)}
        />
      )
    }
    return (
      <ResultCard
        key={card.id}
        card={card}
        onOpen={showAlbum(card)}
        onDownload={onDownloadAlbum(card)}
      />
    )
  }

  const emptyText = () => {
    if (searched) return 'No results'
    if (type === 'artist')
      return 'Search for an artist to browse their releases'
    return `Search YouTube Music for ${type}s to add to your library`
  }

  return (
    <Box className={classes.root}>
      <Title title={translate('menu.discover', { _: 'Discover' })} />
      <div className={classes.searchRow}>
        {type === 'import' ? (
          <>
            <TextField
              className={classes.search}
              placeholder="Playlist, album or track URL"
              value={importLink}
              onChange={(e) => setImportLink(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && onImport()}
            />
            <TextField
              placeholder="Playlist name (optional)"
              value={importName}
              onChange={(e) => setImportName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && onImport()}
            />
            <Button
              variant="contained"
              color="primary"
              onClick={onImport}
              disabled={!importLink.trim() || importing}
              startIcon={
                importing ? (
                  <CircularProgress size={16} color="inherit" />
                ) : undefined
              }
            >
              {importing
                ? 'Importing…'
                : translate('ra.action.import', { _: 'Import' })}
            </Button>
          </>
        ) : (
          <TextField
            className={classes.search}
            placeholder={translate('ra.action.search')}
            value={query}
            onChange={(e) => changeQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && runSearch()}
            InputProps={{
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton
                    onClick={() => runSearch()}
                    size="small"
                    aria-label={translate('ra.action.search')}
                  >
                    <SearchIcon />
                  </IconButton>
                </InputAdornment>
              ),
            }}
          />
        )}
        <Tabs
          value={type}
          onChange={(e, v) => changeType(v)}
          indicatorColor="primary"
          textColor="primary"
        >
          {MODES.map((m) => (
            <Tab key={m.key} value={m.key} label={m.label} />
          ))}
        </Tabs>
      </div>

      <DownloadQueue />

      {type === 'import' && (
        <Typography className={classes.empty}>
          Paste a YouTube Music playlist, album or track URL. Its tracks are
          downloaded and collected into a new playlist.
        </Typography>
      )}

      {loading && (
        <Box textAlign="center" mt={4}>
          <CircularProgress />
        </Box>
      )}

      {!loading && artist && (
        <>
          <Button startIcon={<ArrowBackIcon />} onClick={() => setArtist(null)}>
            {translate('ra.action.back', { _: 'Back' })}
          </Button>
          <Typography variant="h6" className={classes.section}>
            {artist.name}
          </Typography>
          {[
            { key: 'albums', label: 'Albums' },
            { key: 'singles', label: 'Singles' },
          ].map(({ key, label }) =>
            (artist[key] || []).length ? (
              <div key={key}>
                <Typography variant="subtitle2" className={classes.section}>
                  {label}
                </Typography>
                <div className={classes.grid}>
                  {artist[key].map((card) => (
                    <ResultCard
                      key={card.id}
                      card={card}
                      onOpen={showAlbum(card)}
                      onDownload={onDownloadAlbum(card)}
                    />
                  ))}
                </div>
              </div>
            ) : null,
          )}
        </>
      )}

      {!loading &&
        !artist &&
        type !== 'import' &&
        (results.length ? (
          <div className={classes.grid}>{results.map(renderCard)}</div>
        ) : (
          <Typography className={classes.empty}>{emptyText()}</Typography>
        ))}

      <AlbumTracksDialog
        open={albumOpen}
        card={albumCard}
        onClose={() => setAlbumOpen(false)}
        notify={notify}
      />
    </Box>
  )
}

export default Discover

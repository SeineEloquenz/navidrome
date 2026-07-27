import React, { useEffect, useState } from 'react'
import PropTypes from 'prop-types'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  CircularProgress,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemAvatar,
  Avatar,
  Box,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import { completePreview, complete } from './dlClient'

const useStyles = makeStyles((theme) => ({
  header: {
    display: 'flex',
    gap: theme.spacing(2),
    alignItems: 'center',
    marginBottom: theme.spacing(2),
  },
  cover: { width: 72, height: 72 },
  missing: { maxHeight: 220, overflow: 'auto' },
  center: { textAlign: 'center', padding: theme.spacing(3) },
  alt: { marginTop: theme.spacing(2) },
}))

const artistLine = (c) =>
  `${(c.artists || []).join(', ')}${c.year ? ` · ${c.year}` : ''}`

const CompleteAlbumDialog = ({
  open,
  onClose,
  artist,
  album,
  present,
  notify,
}) => {
  const classes = useStyles()
  const [loading, setLoading] = useState(false)
  const [analyzing, setAnalyzing] = useState(false)
  const [error, setError] = useState(null)
  const [candidates, setCandidates] = useState([])
  const [selectedId, setSelectedId] = useState(null)
  const [analysis, setAnalysis] = useState(null) // { totalTracks, missing }
  const [downloading, setDownloading] = useState(false)

  useEffect(() => {
    if (!open) return undefined
    let active = true
    setLoading(true)
    setError(null)
    setCandidates([])
    setAnalysis(null)
    completePreview({ artist, album, present })
      .then((res) => {
        if (!active) return
        setCandidates([res.match, ...(res.alternatives || [])])
        setSelectedId(res.match.id)
        setAnalysis({
          totalTracks: res.match.totalTracks,
          missing: res.match.missing,
        })
      })
      .catch((e) => active && setError(e.message))
      .finally(() => active && setLoading(false))
    return () => {
      active = false
    }
  }, [open, artist, album, present])

  const selectCandidate = async (c) => {
    if (c.id === selectedId) return
    setSelectedId(c.id)
    setAnalyzing(true)
    setError(null)
    try {
      const res = await completePreview({ albumId: c.id, present })
      setAnalysis({
        totalTracks: res.match.totalTracks,
        missing: res.match.missing,
      })
    } catch (e) {
      setError(e.message)
    } finally {
      setAnalyzing(false)
    }
  }

  const onDownload = async () => {
    setDownloading(true)
    try {
      const res = await complete(selectedId, present)
      notify(
        res.missing
          ? `Queued ${res.missing} missing track(s) from "${res.matched}"`
          : 'Album is already complete',
      )
      onClose()
    } catch (e) {
      notify('Failed to complete album: ' + e.message, 'warning')
    } finally {
      setDownloading(false)
    }
  }

  const selected = candidates.find((c) => c.id === selectedId)
  const others = candidates.filter((c) => c.id !== selectedId)
  const missing = analysis?.missing || []

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Get missing tracks</DialogTitle>
      <DialogContent>
        {loading && (
          <div className={classes.center}>
            <CircularProgress />
          </div>
        )}
        {!loading && error && <Typography color="error">{error}</Typography>}
        {!loading && !error && selected && (
          <>
            <Box className={classes.header}>
              {selected.thumbnail && (
                <Avatar
                  variant="rounded"
                  src={selected.thumbnail}
                  className={classes.cover}
                />
              )}
              <div>
                <Typography variant="subtitle1">{selected.title}</Typography>
                <Typography variant="body2" color="textSecondary">
                  {artistLine(selected)}
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  {analyzing
                    ? 'Checking…'
                    : missing.length
                      ? `${missing.length} of ${analysis.totalTracks} track(s) missing`
                      : 'Nothing missing'}
                </Typography>
              </div>
            </Box>
            {!analyzing && missing.length > 0 && (
              <List dense className={classes.missing}>
                {missing.map((t) => (
                  <ListItem key={t.id} disableGutters>
                    <ListItemText primary={`${t.trackNumber}. ${t.title}`} />
                  </ListItem>
                ))}
              </List>
            )}
            {others.length > 0 && (
              <div className={classes.alt}>
                <Typography variant="caption" color="textSecondary">
                  Wrong album? Pick another match:
                </Typography>
                <List dense>
                  {others.map((c) => (
                    <ListItem
                      button
                      key={c.id}
                      onClick={() => selectCandidate(c)}
                    >
                      {c.thumbnail && (
                        <ListItemAvatar>
                          <Avatar variant="rounded" src={c.thumbnail} />
                        </ListItemAvatar>
                      )}
                      <ListItemText
                        primary={c.title}
                        secondary={artistLine(c)}
                      />
                    </ListItem>
                  ))}
                </List>
              </div>
            )}
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={downloading}>
          Cancel
        </Button>
        <Button
          color="primary"
          variant="contained"
          onClick={onDownload}
          disabled={
            loading ||
            analyzing ||
            downloading ||
            !selected ||
            missing.length === 0
          }
        >
          {downloading
            ? 'Queuing…'
            : missing.length
              ? `Download ${missing.length} missing`
              : 'Nothing missing'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

CompleteAlbumDialog.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  artist: PropTypes.string,
  album: PropTypes.string,
  present: PropTypes.arrayOf(PropTypes.number),
  notify: PropTypes.func.isRequired,
}

export default CompleteAlbumDialog

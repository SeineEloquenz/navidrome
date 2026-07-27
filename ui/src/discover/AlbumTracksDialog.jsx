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
  ListItemSecondaryAction,
  IconButton,
  Avatar,
  Chip,
  Box,
  Tooltip,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import GetAppIcon from '@material-ui/icons/GetApp'
import DoneAllIcon from '@material-ui/icons/DoneAll'
import { getAlbum, downloadItems } from './dlClient'

const useStyles = makeStyles((theme) => ({
  header: {
    display: 'flex',
    gap: theme.spacing(2),
    alignItems: 'center',
    marginBottom: theme.spacing(1),
  },
  cover: { width: 72, height: 72 },
  tracks: { maxHeight: 360, overflowY: 'auto' },
  center: { textAlign: 'center', padding: theme.spacing(3) },
  inLibrary: { marginLeft: theme.spacing(1) },
}))

const AlbumTracksDialog = ({ open, onClose, card, notify }) => {
  const classes = useStyles()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [tracks, setTracks] = useState([])
  const [queued, setQueued] = useState({})
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open || !card) return undefined
    let active = true
    setLoading(true)
    setError(null)
    setTracks([])
    setQueued({})
    getAlbum(card.id)
      .then((res) => active && setTracks(res.tracks || []))
      .catch((e) => active && setError(e.message))
      .finally(() => active && setLoading(false))
    return () => {
      active = false
    }
  }, [open, card])

  const queue = async (items, mark) => {
    setBusy(true)
    try {
      const res = await downloadItems(items)
      setQueued((q) => ({ ...q, ...mark }))
      notify(`Queued ${res?.enqueued ?? items.length} download(s)`)
    } catch (e) {
      notify('Download failed: ' + e.message, 'warning')
    } finally {
      setBusy(false)
    }
  }

  const queueTrack = (t) => () => queue([{ videoId: t.id }], { [t.id]: true })

  const queueAll = () => {
    const pending = tracks.filter((t) => !queued[t.id])
    if (!pending.length) return
    queue(
      pending.map((t) => ({ videoId: t.id })),
      Object.fromEntries(pending.map((t) => [t.id, true])),
    )
  }

  const remaining = tracks.filter((t) => !queued[t.id]).length

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{card?.title || 'Album'}</DialogTitle>
      <DialogContent>
        {loading && (
          <div className={classes.center}>
            <CircularProgress />
          </div>
        )}
        {!loading && error && <Typography color="error">{error}</Typography>}
        {!loading && !error && (
          <>
            <Box className={classes.header}>
              {card?.thumbnail && (
                <Avatar
                  variant="rounded"
                  src={card.thumbnail}
                  className={classes.cover}
                />
              )}
              <div>
                <Typography variant="body2" color="textSecondary">
                  {(card?.artists || []).join(', ')}
                  {card?.year ? ` • ${card.year}` : ''}
                </Typography>
                <Typography variant="body2" color="textSecondary">
                  {tracks.length} track(s)
                </Typography>
              </div>
            </Box>
            <List dense className={classes.tracks}>
              {tracks.map((t) => (
                <ListItem key={t.id} disableGutters>
                  <ListItemText
                    primary={
                      <>
                        {t.trackNumber}. {t.title}
                        {t.inLibrary && (
                          <Chip
                            size="small"
                            label="In library"
                            className={classes.inLibrary}
                          />
                        )}
                      </>
                    }
                    secondary={t.duration}
                  />
                  <ListItemSecondaryAction>
                    <Tooltip title={queued[t.id] ? 'Queued' : 'Download track'}>
                      <span>
                        <IconButton
                          edge="end"
                          size="small"
                          aria-label={
                            queued[t.id] ? 'Queued' : `Download ${t.title}`
                          }
                          disabled={busy || !!queued[t.id]}
                          onClick={queueTrack(t)}
                        >
                          {queued[t.id] ? (
                            <DoneAllIcon fontSize="small" />
                          ) : (
                            <GetAppIcon fontSize="small" />
                          )}
                        </IconButton>
                      </span>
                    </Tooltip>
                  </ListItemSecondaryAction>
                </ListItem>
              ))}
            </List>
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
        <Button
          color="primary"
          variant="contained"
          onClick={queueAll}
          disabled={loading || busy || !remaining}
        >
          {remaining ? `Download ${remaining} track(s)` : 'All queued'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

AlbumTracksDialog.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  card: PropTypes.object,
  notify: PropTypes.func.isRequired,
}

export default AlbumTracksDialog

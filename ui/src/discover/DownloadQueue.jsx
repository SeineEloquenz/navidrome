import React, { useState } from 'react'
import { useNotify } from 'react-admin'
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  ListItemSecondaryAction,
  IconButton,
  Button,
  CircularProgress,
  Chip,
  Collapse,
  Tooltip,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import ErrorIcon from '@material-ui/icons/Error'
import LibraryAddCheckIcon from '@material-ui/icons/LibraryAddCheck'
import BlockIcon from '@material-ui/icons/Block'
import ExpandMoreIcon from '@material-ui/icons/ExpandMore'
import ExpandLessIcon from '@material-ui/icons/ExpandLess'
import ScheduleIcon from '@material-ui/icons/Schedule'
import RefreshIcon from '@material-ui/icons/Refresh'
import HelpOutlineIcon from '@material-ui/icons/HelpOutline'
import { useDownloadStatus, refreshDownloadStatus } from './useDownloadStatus'
import { retryDownloads } from './dlClient'

const useStyles = makeStyles((theme) => ({
  panel: {
    marginBottom: theme.spacing(2),
    padding: theme.spacing(1, 2),
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
  },
  header: { display: 'flex', alignItems: 'center', gap: theme.spacing(1) },
  section: { marginTop: theme.spacing(0.5) },
  sectionHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    color: theme.palette.text.secondary,
  },
  finishedHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    cursor: 'pointer',
    color: theme.palette.text.secondary,
  },
  scrollList: { maxHeight: 360, overflowY: 'auto' },
  icon: { minWidth: 32 },
  success: { color: theme.palette.success.main },
  error: { color: theme.palette.error.main },
  muted: { color: theme.palette.text.secondary },
}))

// SomeDL finished statuses -> display.
const FINISHED = {
  success: { label: 'Downloaded', icon: CheckCircleIcon, cls: 'success' },
  already_downloaded: {
    label: 'Already had',
    icon: LibraryAddCheckIcon,
    cls: 'muted',
  },
  download_disabled: { label: 'Skipped', icon: BlockIcon, cls: 'muted' },
  failed: { label: 'Failed', icon: ErrorIcon, cls: 'error' },
}

// An unknown status must not borrow the "Downloaded" label, or a SomeDL that
// stops reporting statuses looks like a run where everything succeeded.
const finishedMeta = (status) =>
  FINISHED[status] || {
    label: status || 'Unknown',
    icon: HelpOutlineIcon,
    cls: 'muted',
  }

const nextTryText = (iso) => {
  const ms = new Date(iso).getTime() - Date.now()
  if (!Number.isFinite(ms)) return 'retry scheduled'
  if (ms <= 0) return 'retrying now'
  const minutes = Math.round(ms / 60000)
  return minutes < 1
    ? 'retrying in under a minute'
    : `retrying in ${minutes} min`
}

const attemptText = (it) => `attempt ${it.retries + 1} of ${it.maxRetries + 1}`

// SomeDL's console.update stage ids -> display. Its own message is more
// specific when it has one, so that wins.
const STAGES = {
  album: 'Looking up album',
  musicbrainz: 'Fetching MusicBrainz data',
  deezer: 'Fetching Deezer data',
  get_lyrics: 'Fetching lyrics',
  label: 'Reading label data',
  wait_queue: 'Waiting in queue',
  downloading: 'Downloading',
  albumart: 'Fetching album art',
  disable_download: 'Downloading is disabled',
}

const stageText = (it) => it.detail || STAGES[it.stage] || it.stage || undefined

const DownloadQueue = () => {
  const classes = useStyles()
  const notify = useNotify()
  const { active, finished, finishedTotal, queued, retrying, failed } =
    useDownloadStatus()
  const [finishedOpen, setFinishedOpen] = useState(false)
  const [retryBusy, setRetryBusy] = useState(false)

  const onRetry = (ids) => async () => {
    setRetryBusy(true)
    try {
      const res = await retryDownloads(ids)
      notify(`Retrying ${res?.restarted ?? 0} download(s)`)
      await refreshDownloadStatus()
    } catch (e) {
      notify('Retry failed: ' + e.message, 'warning')
    } finally {
      setRetryBusy(false)
    }
  }

  if (
    !active.length &&
    !queued &&
    !finished.length &&
    !retrying.length &&
    !failed.length
  )
    return null

  return (
    <Box className={classes.panel}>
      <div className={classes.header}>
        <Typography variant="subtitle2">Downloads</Typography>
        {queued > 0 && <Chip size="small" label={`${queued} queued`} />}
      </div>

      {active.length > 0 && (
        <List dense disablePadding className={classes.section}>
          {active.map((it) => (
            <ListItem key={it.id} disableGutters>
              <ListItemIcon className={classes.icon}>
                <CircularProgress size={16} />
              </ListItemIcon>
              <ListItemText
                primary={it.label || 'Downloading…'}
                secondary={stageText(it)}
              />
            </ListItem>
          ))}
        </List>
      )}

      {retrying.length > 0 && (
        <div className={classes.section}>
          <div className={classes.sectionHeader}>
            <Typography variant="caption">
              Waiting to retry ({retrying.length})
            </Typography>
          </div>
          <List dense disablePadding className={classes.scrollList}>
            {retrying.map((it) => (
              <ListItem key={it.id} disableGutters>
                <ListItemIcon className={classes.icon}>
                  <ScheduleIcon fontSize="small" className={classes.muted} />
                </ListItemIcon>
                <ListItemText
                  primary={it.label || 'Failed download'}
                  secondary={`${attemptText(it)}, ${nextTryText(it.nextTry)}`}
                />
              </ListItem>
            ))}
          </List>
        </div>
      )}

      {failed.length > 0 && (
        <div className={classes.section}>
          <div className={classes.sectionHeader}>
            <Typography variant="caption" className={classes.error}>
              Gave up ({failed.length})
            </Typography>
            <Button
              size="small"
              startIcon={<RefreshIcon />}
              disabled={retryBusy}
              onClick={onRetry(undefined)}
            >
              Retry all
            </Button>
          </div>
          <List dense disablePadding className={classes.scrollList}>
            {failed.map((it) => (
              <ListItem key={it.id} disableGutters>
                <ListItemIcon className={classes.icon}>
                  <ErrorIcon fontSize="small" className={classes.error} />
                </ListItemIcon>
                <ListItemText
                  primary={it.label || 'Failed download'}
                  secondary={`gave up after ${it.retries + 1} attempts`}
                />
                <ListItemSecondaryAction>
                  <Tooltip title="Retry">
                    <span>
                      <IconButton
                        edge="end"
                        size="small"
                        disabled={retryBusy}
                        onClick={onRetry([it.id])}
                      >
                        <RefreshIcon fontSize="small" />
                      </IconButton>
                    </span>
                  </Tooltip>
                </ListItemSecondaryAction>
              </ListItem>
            ))}
          </List>
        </div>
      )}

      {finished.length > 0 && (
        <div className={classes.section}>
          <div
            className={classes.finishedHeader}
            onClick={() => setFinishedOpen((v) => !v)}
          >
            {finishedOpen ? (
              <ExpandLessIcon fontSize="small" />
            ) : (
              <ExpandMoreIcon fontSize="small" />
            )}
            <Typography variant="caption">
              {finished.length < finishedTotal
                ? `Finished (showing ${finished.length} of ${finishedTotal})`
                : `Finished (${finished.length})`}
            </Typography>
          </div>
          <Collapse in={finishedOpen}>
            <List dense disablePadding className={classes.scrollList}>
              {finished.map((it) => {
                const meta = finishedMeta(it.status)
                const Icon = meta.icon
                return (
                  <ListItem key={it.id} disableGutters>
                    <ListItemIcon className={classes.icon}>
                      <Icon fontSize="small" className={classes[meta.cls]} />
                    </ListItemIcon>
                    <ListItemText
                      primary={it.label || meta.label}
                      secondary={it.label ? meta.label : undefined}
                    />
                  </ListItem>
                )
              })}
            </List>
          </Collapse>
        </div>
      )}
    </Box>
  )
}

export default DownloadQueue

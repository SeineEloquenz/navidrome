import React, { useState } from 'react'
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  CircularProgress,
  Chip,
  Collapse,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import ErrorIcon from '@material-ui/icons/Error'
import LibraryAddCheckIcon from '@material-ui/icons/LibraryAddCheck'
import BlockIcon from '@material-ui/icons/Block'
import ExpandMoreIcon from '@material-ui/icons/ExpandMore'
import ExpandLessIcon from '@material-ui/icons/ExpandLess'
import { useDownloadStatus } from './useDownloadStatus'

const useStyles = makeStyles((theme) => ({
  panel: {
    marginBottom: theme.spacing(2),
    padding: theme.spacing(1, 2),
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
  },
  header: { display: 'flex', alignItems: 'center', gap: theme.spacing(1) },
  section: { marginTop: theme.spacing(0.5) },
  finishedHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    cursor: 'pointer',
    color: theme.palette.text.secondary,
  },
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

const DownloadQueue = () => {
  const classes = useStyles()
  const { active, finished, queued } = useDownloadStatus()
  const [finishedOpen, setFinishedOpen] = useState(false)

  if (!active.length && !queued && !finished.length) return null

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
              <ListItemText primary={it.label || 'Downloading…'} />
            </ListItem>
          ))}
        </List>
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
              Downloaded ({finished.length})
            </Typography>
          </div>
          <Collapse in={finishedOpen}>
            <List dense disablePadding>
              {finished.map((it) => {
                const meta = FINISHED[it.status] || FINISHED.success
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

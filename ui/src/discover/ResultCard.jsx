import React, { useState } from 'react'
import {
  Card,
  CardActionArea,
  CardMedia,
  CardContent,
  Typography,
  IconButton,
  CircularProgress,
  Tooltip,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import GetAppIcon from '@material-ui/icons/GetApp'
import CheckIcon from '@material-ui/icons/Check'
import DoneAllIcon from '@material-ui/icons/DoneAll'

const useStyles = makeStyles((theme) => ({
  card: { position: 'relative' },
  media: { paddingTop: '100%', backgroundColor: theme.palette.action.hover },
  content: { padding: theme.spacing(1) },
  title: {
    fontWeight: 600,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  subtitle: {
    color: theme.palette.text.secondary,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  action: {
    position: 'absolute',
    top: theme.spacing(1),
    right: theme.spacing(1),
    display: 'inline-flex',
  },
  actionButton: {
    backgroundColor: 'rgba(0,0,0,0.55)',
    color: '#fff',
    '&:hover': { backgroundColor: 'rgba(0,0,0,0.75)' },
    '&.Mui-disabled': { backgroundColor: 'rgba(0,0,0,0.4)', color: '#fff' },
  },
  badge: {
    position: 'absolute',
    top: theme.spacing(1),
    left: theme.spacing(1),
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    padding: theme.spacing(0.25, 0.75),
    borderRadius: theme.shape.borderRadius,
    fontSize: '0.7rem',
    lineHeight: 1.6,
    backgroundColor: theme.palette.success.main,
    color: theme.palette.success.contrastText,
  },
}))

const subtitle = (card) => {
  const artists = (card.artists || []).join(', ')
  const meta = card.year || card.duration
  return [artists, meta].filter(Boolean).join(' • ')
}

const ResultCard = ({ card, onOpen, onDownload }) => {
  const classes = useStyles()
  const [busy, setBusy] = useState(false)
  const [queued, setQueued] = useState(false)

  const handleDownload = async (e) => {
    e.stopPropagation()
    setBusy(true)
    try {
      await onDownload()
      setQueued(true)
    } finally {
      setBusy(false)
    }
  }

  // In-library stays clickable: SomeDL skips tracks already on disk, so this is
  // how a partly-owned album gets filled in from search.
  const hint = queued
    ? 'Queued'
    : card.inLibrary
      ? 'In library, download any missing tracks'
      : `Download ${card.title}`

  return (
    <Card className={classes.card}>
      {card.inLibrary && (
        <span className={classes.badge}>
          <CheckIcon style={{ fontSize: '0.85rem' }} />
          In library
        </span>
      )}
      <CardActionArea onClick={onOpen} disabled={!onOpen}>
        {card.thumbnail ? (
          <CardMedia className={classes.media} image={card.thumbnail} />
        ) : (
          <div className={classes.media} />
        )}
        <CardContent className={classes.content}>
          <Typography className={classes.title} variant="body2">
            {card.title}
          </Typography>
          <Typography className={classes.subtitle} variant="caption">
            {subtitle(card)}
          </Typography>
        </CardContent>
      </CardActionArea>
      {onDownload && (
        <Tooltip title={hint}>
          <span className={classes.action}>
            <IconButton
              size="small"
              className={classes.actionButton}
              aria-label={hint}
              onClick={handleDownload}
              disabled={busy || queued}
            >
              {busy ? (
                <CircularProgress size={18} color="inherit" />
              ) : queued ? (
                <DoneAllIcon fontSize="small" />
              ) : (
                <GetAppIcon fontSize="small" />
              )}
            </IconButton>
          </span>
        </Tooltip>
      )}
    </Card>
  )
}

export default ResultCard

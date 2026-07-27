import React, { useState } from 'react'
import {
  Card,
  CardActionArea,
  CardMedia,
  CardContent,
  Typography,
  IconButton,
  CircularProgress,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import GetAppIcon from '@material-ui/icons/GetApp'
import CheckIcon from '@material-ui/icons/Check'

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
    backgroundColor: 'rgba(0,0,0,0.55)',
    color: '#fff',
    '&:hover': { backgroundColor: 'rgba(0,0,0,0.75)' },
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
  const parts = [...(card.artists || [])]
  const meta = card.year || card.duration
  if (meta) parts.push(meta)
  return parts.join(' • ')
}

const ResultCard = ({ card, onOpen, onDownload }) => {
  const classes = useStyles()
  const [busy, setBusy] = useState(false)

  const handleDownload = async (e) => {
    e.stopPropagation()
    setBusy(true)
    try {
      await onDownload()
    } finally {
      setBusy(false)
    }
  }

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
        <IconButton
          className={classes.action}
          size="small"
          onClick={handleDownload}
          disabled={busy || card.inLibrary}
        >
          {busy ? (
            <CircularProgress size={18} color="inherit" />
          ) : (
            <GetAppIcon fontSize="small" />
          )}
        </IconButton>
      )}
    </Card>
  )
}

export default ResultCard

import React from 'react'
import PropTypes from 'prop-types'
import { useDispatch } from 'react-redux'
import {
  Button,
  sanitizeListRestProps,
  TopToolbar,
  useNotify,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import { useMediaQuery, makeStyles } from '@material-ui/core'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import ShuffleIcon from '@material-ui/icons/Shuffle'
import CloudDownloadOutlinedIcon from '@material-ui/icons/CloudDownloadOutlined'
import LibraryAddIcon from '@material-ui/icons/LibraryAdd'
import { RiPlayListAddFill, RiPlayList2Fill } from 'react-icons/ri'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import ShareIcon from '@material-ui/icons/Share'
import {
  playNext,
  addTracks,
  playTracks,
  shuffleTracks,
  openAddToPlaylist,
  openDownloadMenu,
  DOWNLOAD_MENU_ALBUM,
  openShareMenu,
} from '../actions'
import { formatBytes } from '../utils'
import config from '../config'
import { ToggleFieldsMenu } from '../common'
import CompleteAlbumDialog from '../discover/CompleteAlbumDialog'

const useStyles = makeStyles({
  toolbar: { display: 'flex', justifyContent: 'space-between', width: '100%' },
})

const AlbumButton = ({ children, ...rest }) => {
  const record = useRecordContext(rest) || {}
  return (
    <Button {...rest} disabled={record.missing}>
      {children}
    </Button>
  )
}

const AlbumActions = ({
  className,
  ids,
  data,
  record,
  permanentFilter,
  ...rest
}) => {
  const dispatch = useDispatch()
  const translate = useTranslate()
  const notify = useNotify()
  const classes = useStyles()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const [completeOpen, setCompleteOpen] = React.useState(false)

  const present = React.useMemo(
    () =>
      ids.map((id) => data[id]?.trackNumber).filter((n) => Number.isInteger(n)),
    [ids, data],
  )

  const handlePlay = React.useCallback(() => {
    dispatch(playTracks(data, ids))
  }, [dispatch, data, ids])

  const handlePlayNext = React.useCallback(() => {
    dispatch(playNext(data, ids))
  }, [dispatch, data, ids])

  const handlePlayLater = React.useCallback(() => {
    dispatch(addTracks(data, ids))
  }, [dispatch, data, ids])

  const handleShuffle = React.useCallback(() => {
    dispatch(shuffleTracks(data, ids))
  }, [dispatch, data, ids])

  const handleAddToPlaylist = React.useCallback(() => {
    const selectedIds = ids.filter((id) => !data[id].missing)
    dispatch(openAddToPlaylist({ selectedIds }))
  }, [dispatch, data, ids])

  const handleShare = React.useCallback(() => {
    dispatch(openShareMenu([record.id], 'album', record.name))
  }, [dispatch, record])

  const handleDownload = React.useCallback(() => {
    dispatch(openDownloadMenu(record, DOWNLOAD_MENU_ALBUM))
  }, [dispatch, record])

  const handleComplete = React.useCallback(() => setCompleteOpen(true), [])

  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      <div className={classes.toolbar}>
        <div>
          <AlbumButton
            onClick={handlePlay}
            label={translate('resources.album.actions.playAll')}
          >
            <PlayArrowIcon />
          </AlbumButton>
          <AlbumButton
            onClick={handleShuffle}
            label={translate('resources.album.actions.shuffle')}
          >
            <ShuffleIcon />
          </AlbumButton>
          <AlbumButton
            onClick={handlePlayNext}
            label={translate('resources.album.actions.playNext')}
          >
            <RiPlayList2Fill />
          </AlbumButton>
          <AlbumButton
            onClick={handlePlayLater}
            label={translate('resources.album.actions.addToQueue')}
          >
            <RiPlayListAddFill />
          </AlbumButton>
          <AlbumButton
            onClick={handleAddToPlaylist}
            label={translate('resources.album.actions.addToPlaylist')}
          >
            <PlaylistAddIcon />
          </AlbumButton>
          {config.enableSharing && (
            <AlbumButton
              onClick={handleShare}
              label={translate('ra.action.share')}
            >
              <ShareIcon />
            </AlbumButton>
          )}
          {config.enableDownloads && (
            <AlbumButton
              onClick={handleDownload}
              label={
                translate('ra.action.download') +
                (isDesktop ? ` (${formatBytes(record.size)})` : '')
              }
            >
              <CloudDownloadOutlinedIcon />
            </AlbumButton>
          )}
          <AlbumButton
            onClick={handleComplete}
            label={translate('resources.album.actions.completeAlbum', {
              _: 'Get missing tracks',
            })}
          >
            <LibraryAddIcon />
          </AlbumButton>
        </div>
        <div>{isNotSmall && <ToggleFieldsMenu resource="albumSong" />}</div>
      </div>
      <CompleteAlbumDialog
        open={completeOpen}
        onClose={() => setCompleteOpen(false)}
        artist={record.albumArtist}
        album={record.name}
        present={present}
        notify={notify}
      />
    </TopToolbar>
  )
}

AlbumActions.propTypes = {
  record: PropTypes.object.isRequired,
  selectedIds: PropTypes.arrayOf(PropTypes.number),
}

AlbumActions.defaultProps = {
  record: {},
  selectedIds: [],
}

export default AlbumActions

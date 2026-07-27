import React from 'react'
import PropTypes from 'prop-types'
import { MenuItemLink, useTranslate } from 'react-admin'
import { Badge } from '@material-ui/core'
import ExploreIcon from '@material-ui/icons/Explore'
import { useDownloadStatus } from './useDownloadStatus'

// The Discover sidebar entry, badged with the number of downloads still in
// flight (active, queued, or waiting on a retry), falling back to the count of
// given-up downloads so failures aren't hidden. Badge hides at 0.
const DiscoverMenuItem = ({ sidebarIsOpen, dense, activeClassName }) => {
  const translate = useTranslate()
  const { active, queued, retrying, failed } = useDownloadStatus()
  const inFlight = active.length + queued + retrying.length
  return (
    <MenuItemLink
      to="/discover"
      primaryText={translate('menu.discover', { _: 'Discover' })}
      leftIcon={
        <Badge
          badgeContent={inFlight || failed.length}
          color={inFlight ? 'primary' : 'error'}
        >
          <ExploreIcon />
        </Badge>
      }
      activeClassName={activeClassName}
      sidebarIsOpen={sidebarIsOpen}
      dense={dense}
    />
  )
}

DiscoverMenuItem.propTypes = {
  sidebarIsOpen: PropTypes.bool,
  dense: PropTypes.bool,
  activeClassName: PropTypes.string,
}

export default DiscoverMenuItem

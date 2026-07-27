import React from 'react'
import PropTypes from 'prop-types'
import { MenuItemLink, useTranslate } from 'react-admin'
import { Badge } from '@material-ui/core'
import ExploreIcon from '@material-ui/icons/Explore'
import { useDownloadStatus } from './useDownloadStatus'

// The Discover sidebar entry, badged with the number of downloads still
// active/queued so it's visible from any page. Badge hides at 0.
const DiscoverMenuItem = ({ sidebarIsOpen, dense, activeClassName }) => {
  const translate = useTranslate()
  const { active, queued } = useDownloadStatus()
  return (
    <MenuItemLink
      to="/discover"
      primaryText={translate('menu.discover', { _: 'Discover' })}
      leftIcon={
        <Badge badgeContent={active.length + queued} color="primary">
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

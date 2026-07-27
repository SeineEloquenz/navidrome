import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import Discover from './discover/Discover'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route exact path="/discover" render={() => <Discover />} key={'discover'} />,
]

export default routes

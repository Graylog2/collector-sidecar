import React from 'react';
import jsRoutes from 'routing/jsRoutes';
import { LinkContainer } from 'react-router-bootstrap';

import {Timestamp} from 'components/common';

const CollectorRow = React.createClass({
    propTypes: {
        collector: React.PropTypes.object.isRequired,
    },
    getInitialState() {
        return {
            showRelativeTime: true,
        };
    },
    render() {
        const collector = this.props.collector;
        const collectorClass = collector.active ? '' : 'greyed-out inactive';
        const style = {};
        return (
            <tr className={collectorClass} style={style}>
                <td className="limited">
                    <LinkContainer to={`/collector/${collector.id}`}>
                        <a>{collector.node_id}</a>
                    </LinkContainer>
                </td>
                <td className="limited">
                    <Timestamp dateTime={collector.last_seen} relative={this.state.showRelativeTime}/>
                </td>
                <td className="limited">
                    <a href={jsRoutes.controllers.SearchController.index('gl2_source_collector:' + collector.id, 'relative', 28800).url}
                       className="btn btn-info btn-xs">Show messages from this collector</a>
                </td>
            </tr>
        );
    },
});

export default CollectorRow;
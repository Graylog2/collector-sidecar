import React from 'react';
import Reflux from 'reflux';
import { Button, Row, Col, Alert } from 'react-bootstrap';

import {Spinner} from 'components/common';

import CollectorsStore from './CollectorsStore';
import CollectorRow from './CollectorRow';
import CollectorsActions from './CollectorsActions';

const CollectorsList = React.createClass({
    mixins: [Reflux.connect(CollectorsStore)],

    getInitialState() {
        return {
            filter: '',
            sort: undefined,
            showInactive: false,
        };
    },

    componentDidMount() {
        CollectorsActions.list();
        this.timeoutID = setTimeout(CollectorsActions.list, 5000);
    },

    componentWillUnmount() {
        clearTimeout(this.timeoutID);
    },

    _formatEmptyListAlert() {
        const showInactiveHint = (this.state.showInactive ? null : ' and/or click on \"Include inactive collectors\"');
        return <Alert>There are no collectors to show. Try adjusting your search filter{showInactiveHint}.</Alert>;
    },

    _getFilteredCollectors() {
        const filter = this.state.filter.toLowerCase().trim();
        return this.state.collectors.filter((collector) => { return !filter || collector.collector_id.toLowerCase().indexOf(filter) !== -1; });
    },

    _formatCollectorList(collectors) {
        return (
            <div className="table-responsive">
                <table className="table table-striped collectors-list">
                    <thead>
                    <tr>
                        <th onClick={this.sortByCollectorId}>Collector</th>
                        <th onClick={this.sortByLastSeen}>Last Seen</th>
                        <th style={{width: 170}}>Action</th>
                    </tr>
                    </thead>
                    <tbody>
                    {collectors}
                    </tbody>
                </table>
            </div>
        );
    },

    _isLoading() {
        return !(this.state.collectors);
    },

    render() {
        if (this._isLoading()) {
            return <Spinner/>;
        }

        if (this.state.collectors) {
            const collectors = this._getFilteredCollectors()
                .filter((collector) => {return (this.state.showInactive || collector.active);})
                .sort(this._bySortField)
                .map((collector) => {
                        return <CollectorRow key={collector.id} collector={collector}/>;
                    }
                );

            const showOrHideInactive = (this.state.showInactive ? 'Hide' : 'Include');

            const collectorList = (collectors.length > 0 ? this._formatCollectorList(collectors) : this._formatEmptyListAlert());

            return (
                <Row className="content">
                    <Col md={12}>
                        <a onClick={this.toggleShowInactive} className="btn btn-primary pull-right">{showOrHideInactive} inactive collectors</a>

                        <form className="form-inline collectors-filter-form" onSubmit={(evt) => evt.preventDefault() }>
                            <div className="form-group form-group-sm">
                                <label htmlFor="collectorsfilter" className="control-label">Filter collectors:</label>
                                <input type="text" name="filter" id="collectorsfilter" className="form-control" value={this.state.filter} onChange={(event) => {this.setState({filter: event.target.value});}} />
                            </div>
                        </form>

                        {collectorList}
                    </Col>
                </Row>
            );
        }

        },
});

export default CollectorsList;

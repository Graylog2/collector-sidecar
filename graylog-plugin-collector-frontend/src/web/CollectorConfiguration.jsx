import React from 'react';
import Reflux from 'reflux';
import { Row, Col } from 'react-bootstrap';

import {DataTable, PageHeader, Spinner} from 'components/common';

import EditInputModal from './EditInputModal';
import EditOutputModal from './EditOutputModal';
import DeleteInputButton from './DeleteInputButton'
import CollectorsActions from './CollectorsActions';
import CollectorsStore from './CollectorsStore';

const CollectorConfiguration = React.createClass({
    getInitialState() {
        return {
            inputs: undefined,
            outputs: undefined,
        };
    },

    componentDidMount() {
        CollectorsActions.getConfiguration.triggerPromise(this.props.params.id).then(this._setConfiguration);
    },

    _setConfiguration(configuration) {
        this.setState({inputs: configuration.inputs, outputs: configuration.outputs});
    },

    _headerCellFormatter(header) {
        let formattedHeaderCell;

        switch (header.toLocaleLowerCase()) {
            case 'name':
                formattedHeaderCell = <th className="name">{header}</th>;
                break;
            case 'type':
                formattedHeaderCell = <th className="type">{header}</th>;
                break;
            case 'forward_to':
                formattedHeaderCell = <th className="forwardTo">{header}</th>;
                break;
            default:
                formattedHeaderCell = <th>{header}</th>;
        }

        return formattedHeaderCell;
    },

    _inputFormatter(input) {
        return (
            <tr key={input.input_id}>
                <td>{input.name}</td>
                <td>{input.type}</td>
                <td>{input.forward_to}</td>
                <td>{JSON.stringify(input.properties)}</td>
                <td style={{width: 130}}><EditInputModal id={input.input_id} name={input.name}
                                                        properties={JSON.stringify(input.properties)} create={false}
                                                        reload={this.loadData} savePattern={this._saveInput}
                                                        validPatternName={this.validInputName}/>
                                        <DeleteInputButton input={input} onClick={this._deleteInput}/></td>
            </tr>
        );
    },

    _outputFormatter(output) {
        return (
            <tr key={output.input_id}>
                <td>{output.name}</td>
                <td>{output.type}</td>
                <td>{JSON.stringify(output.properties)}</td>
                <td style={{width: 70}}><EditInputModal id={output.output_id} name={output.name}
                                                        properties={JSON.stringify(output.properties)} create={false}
                                                        reload={this.loadData} savePattern={this._saveOutput}
                                                        validPatternName={this.validInputName}/></td>
            </tr>
        );
    },

    _saveInput(input, callback) {
        CollectorsStore.saveInput(input, this.props.params.id, () => {
            callback();
        });
    },

    _deleteInput(input, callback) {
        CollectorsStore.deleteInput(input, this.props.params.id, () => {
            callback();
        });
    },

    _saveOutput(output, callback) {
        CollectorsStore.saveOutput(output, this.props.params.id, () => {
            callback();
        });
    },

    _validOutputName(name) {
        // Check if outputs already contain an output with the given name.
        return !this.state.outputs.some((output) => output.name === name);
    },

    _validInputName(name) {
        // Check if inputs already contain an input with the given name.
        return !this.state.inputs.some((input) => input.name === name);
    },

    _isLoading() {
        return !(this.state.inputs  && this.state.outputs);
    },

    render() {
        const inputHeaders = ['Input', 'Type', 'Forward To', 'Properties', 'Actions'];
        const outputHeaders = ['Output', 'Type', 'Properties', 'Actions'];
        const filterKeys = [];

        if (this._isLoading()) {
            return <Spinner/>;
        }

        return(
            <div>
                <PageHeader title="Collector Configuration" titleSize={8} buttonSize={4} buttonStyle={{textAlign: 'right', marginTop: '10px'}}>
                    <span>
                        This is a list of inputs and outputs configured for the current collector.
                    </span>
                    {null}
                </PageHeader>
                <Row className="content">
                    <Col md={12}>
                        <div className="pull-right">
                            <EditOutputModal id={""} name={""} properties={""} create
                                            reload={this.loadData}
                                            saveOutput={this._saveOutput}
                                            validOutputName={this._validOutputName}/>
                        </div>
                        <DataTable id="collector-outputs-list"
                                   className="table-striped table-hover"
                                   headers={outputHeaders}
                                   headerCellFormatter={this._headerCellFormatter}
                                   sortByKey={"type"}
                                   rows={this.state.outputs}
                                   dataRowFormatter={this._outputFormatter}
                                   filterLabel="Filter patterns"
                                   filterKeys={filterKeys}/>
                    </Col>
                </Row>
                <Row className="content">
                    <Col md={12}>
                        <div className="pull-right">
                            <EditInputModal id={""} name={""} properties={""} create
                                            reload={this.loadData}
                                            saveInput={this._saveInput}
                                            validInputName={this._validInputName}/>
                        </div>
                        <DataTable id="collector-inputs-list"
                                   className="table-striped table-hover"
                                   headers={inputHeaders}
                                   headerCellFormatter={this._headerCellFormatter}
                                   sortByKey={"type"}
                                   rows={this.state.inputs}
                                   dataRowFormatter={this._inputFormatter}
                                   filterLabel="Filter patterns"
                                   filterKeys={filterKeys}/>
                    </Col>
                </Row>
            </div>
        )
    },
});

export default CollectorConfiguration;
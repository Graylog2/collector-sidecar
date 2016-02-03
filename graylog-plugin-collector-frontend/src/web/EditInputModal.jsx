import React from 'react';
import { Input } from 'react-bootstrap';

import BootstrapModalForm from 'components/bootstrap/BootstrapModalForm';
import { KeyValueTable, Select } from 'components/common';

import CollectorsActions from './CollectorsActions';
//import KVTable from './KVTable'

const EditInputModal = React.createClass({
    propTypes: {
        id: React.PropTypes.string,
        name: React.PropTypes.string,
        forwardto: React.PropTypes.string,
        properties: React.PropTypes.object,
        outputs: React.PropTypes.array,
        create: React.PropTypes.bool,
        saveInput: React.PropTypes.func.isRequired,
        validInputName: React.PropTypes.func.isRequired,
    },

    getInitialState() {
        return {
            id: this.props.id,
            name: this.props.name,
            forwardto: this.props.forwardto,
            properties: this.props.properties,
            error: false,
            error_message: '',
        };
    },

    openModal() {
        this.refs.modal.open();
    },

    _getId(prefixIdName) {
        return this.state.name !== undefined ? prefixIdName + this.state.name : prefixIdName;
    },

    _closeModal() {
        this.refs.modal.close();
    },

    _saved() {
        this._closeModal();
        if (this.props.create) {
            this.setState({name: '', forwardto: '', properties: {}});
        }
    },

    _save() {
        const configuration = this.state;

        if (!configuration.error) {
            this.props.saveInput(configuration, this._saved);
        }
    },

    _changeName(event) {
        this.setState({name: event.target.value});
    },

    _changeForwardtoDropdown(selectedValue) {
        this.setState({forwardto: selectedValue});
    },

    _changeProperties(properties) {
        this.setState({properties: properties});
    },

    _formatDropdownOptions() {
        let options = [];

        if (this.props.outputs) {
            var outputCount = this.props.outputs.length;
            for (var i = 0; i < outputCount; i++) {
                options.push({value: this.props.outputs[i].name, label: this.props.outputs[i].name});
            }
        } else {
            options.push({value: 'none', label: 'No outputs available', disable: true});
        }

        return options;
    },

    render() {
        let triggerButtonContent;
        if (this.props.create) {
            triggerButtonContent = 'Create input';
        } else {
            triggerButtonContent = <span>Edit</span>;
        }

        return (
        <span>
                <button onClick={this.openModal}
                        className={this.props.create ? 'btn btn-success' : 'btn btn-info btn-xs'}>
                    {triggerButtonContent}
                </button>
                <BootstrapModalForm ref="modal"
                                    title={`${this.props.create ? 'Create' : 'Edit'} Input ${this.state.name}`}
                                    onSubmitForm={this._save}
                                    submitButtonText="Save">
                    <fieldset>
                        <Input type="text"
                               id={this._getId('input-name')}
                               label="Name"
                               defaultValue={this.state.name}
                               onChange={this._changeName}
                               bsStyle={this.state.error ? 'error' : null}
                               help={this.state.error ? this.state.error_message : null}
                               autoFocus
                               required/>
                        <Select placeholder="Forward to output"
                                options={this._formatDropdownOptions()} matchProp="label"
                                onValueChange={this._changeForwardtoDropdown} value={''} />
                        <KeyValueTable pairs={this.state.properties} onChange={this._changeProperties} editable />
                    </fieldset>
                </BootstrapModalForm>
            </span>
    );
    },
});

export default EditInputModal;

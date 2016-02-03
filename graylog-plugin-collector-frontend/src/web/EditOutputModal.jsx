import React from 'react';
import { Input } from 'react-bootstrap';

import BootstrapModalForm from 'components/bootstrap/BootstrapModalForm';
import { KeyValueTable } from 'components/common';

const EditOutputModal = React.createClass({
    propTypes: {
        id: React.PropTypes.string,
        name: React.PropTypes.string,
        properties: React.PropTypes.object,
        create: React.PropTypes.bool,
        saveOutput: React.PropTypes.func.isRequired,
        validOutputName: React.PropTypes.func.isRequired,
    },

    getInitialState() {
        return {
            id: this.props.id,
            name: this.props.name,
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
            this.setState({name: '', properties: ''});
        }
    },

    _save() {
        const configuration = this.state;

        if (!configuration.error) {
            this.props.saveOutput(configuration, this._saved);
        }
    },

    _changeName(event) {
        this.setState({name: event.target.value})
    },

    _changeProperties(properties) {
        this.setState({properties: properties})
    },

    render() {
        let triggerButtonContent;
        if (this.props.create) {
            triggerButtonContent = 'Create output';
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
                                    title={`${this.props.create ? 'Create' : 'Edit'} Output ${this.state.name}`}
                                    onSubmitForm={this._save}
                                    submitButtonText="Save">
                    <fieldset>
                        <Input type="text"
                               id={this._getId('output-name')}
                               label="Name"
                               defaultValue={this.state.name}
                               onChange={this._changeName}
                               bsStyle={this.state.error ? 'error' : null}
                               help={this.state.error ? this.state.error_message : null}
                               autoFocus
                               required/>
                        <KeyValueTable pairs={this.state.properties} onChange={this._changeProperties} editable />
                    </fieldset>
                </BootstrapModalForm>
            </span>
    );
  },
});

export default EditOutputModal;

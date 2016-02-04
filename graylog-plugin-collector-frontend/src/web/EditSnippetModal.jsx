import React from 'react';
import { Input } from 'react-bootstrap';

import BootstrapModalForm from 'components/bootstrap/BootstrapModalForm';
import { KeyValueTable, Select } from 'components/common';

import CollectorsActions from './CollectorsActions';

const EditSnippetModal = React.createClass({
    propTypes: {
        id: React.PropTypes.string,
        name: React.PropTypes.string,
        type: React.PropTypes.string,
        snippet: React.PropTypes.string,
        create: React.PropTypes.bool,
        saveSnippet: React.PropTypes.func.isRequired,
        validSnippetName: React.PropTypes.func.isRequired,
    },

    getInitialState() {
        return {
            id: this.props.id,
            name: this.props.name,
            type: this.props.type,
            snippet: this.props.snippet,
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
            this.setState({name: '', type: '', snippet: ''});
        }
    },

    _save() {
        const configuration = this.state;

        if (!configuration.error) {
            this.props.saveSnippet(configuration, this._saved);
        }
    },

    _changeName(event) {
        this.setState({name: event.target.value});
    },

    _changeType(event) {
        this.setState({type: event.target.value});
    },

    _changeSnippet(event) {
        this.setState({snippet: event.target.value});
    },

    render() {
        let triggerButtonContent;
        if (this.props.create) {
            triggerButtonContent = 'Create snippet';
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
                                    title={`${this.props.create ? 'Create' : 'Edit'} Snippet ${this.state.name}`}
                                    onSubmitForm={this._save}
                                    submitButtonText="Save">
                    <fieldset>
                        <Input type="text"
                               id={this._getId('snippet-name')}
                               label="Name"
                               defaultValue={this.state.name}
                               onChange={this._changeName}
                               bsStyle={this.state.error ? 'error' : null}
                               help={this.state.error ? this.state.error_message : null}
                               autoFocus
                               required/>
                        <Input type="textarea"
                               id={this._getId('snippet-content')}
                               label="Snippet"
                               defaultValue={this.state.snippet}
                               onChange={this._changeSnippet}
                               bsStyle={this.state.error ? 'error' : null}
                               help={this.state.error ? this.state.error_message : null}
                               required/>
                    </fieldset>
                </BootstrapModalForm>
            </span>
        );
    },
});

export default EditSnippetModal;
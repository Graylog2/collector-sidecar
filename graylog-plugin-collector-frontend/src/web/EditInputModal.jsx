import React from 'react';
import { Input } from 'react-bootstrap';

import BootstrapModalForm from 'components/bootstrap/BootstrapModalForm';
import PropertiesTable from 'PropertiesTable';

const EditInputModal = React.createClass({
    propTypes: {
        id: React.PropTypes.string,
        name: React.PropTypes.string,
        forwardto: React.PropTypes.string,
        properties: React.PropTypes.string,
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
            this.setState({name: '', forwardto: '', properties: ''});
        }
    },

    _save() {
        const configuration = this.state;

        if (!configuration.error) {
            this.props.saveInput(configuration, this._saved);
        }
    },

    _changeName(event) {
        this.setState({name: event.target.value})
    },

    _changeForwardto(event) {
        this.setState({forwardto: event.target.value})
    },

    _changeProperties(event) {
        this.setState({properties: event.target.value})
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
                        <Input type="text"
                               id={this._getId('input-forward-to')}
                               label="Forward To"
                               defaultValue={this.state.forwardto}
                               onChange={this._changeForwardto}
                               bsStyle={this.state.error ? 'error' : null}
                               help={this.state.error ? this.state.error_message : null}
                               required/>
                        <Input type="textarea"
                               id={this._getId('input-properties')}
                               label="Properties"
                               defaultValue={this.state.properties}
                               onChange={this._changeProperties}
                               required/>
                        <PropertiesTable properties={[]}/>
                    </fieldset>
                </BootstrapModalForm>
            </span>
    );
  },
});

export default EditInputModal;

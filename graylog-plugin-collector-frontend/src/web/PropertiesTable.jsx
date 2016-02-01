import React from 'react';
import ReactDOM from 'react-dom';

import { Button, Input } from 'react-bootstrap';

const PropertiesTable = React.createClass({
    propTypes: {
        properties: React.PropTypes.array.isRequired,
    },

    getInitialState() {
        return {
            properties: [{key: "key", value: "value"}],
        };
    },

    handleNewRowSubmit( newproperty ) {
        this.setState( {properties: this.state.properties.concat([newproperty])} );
    },

    handleRowRemove( property ) {
        var index = -1;
        var plength = this.state.properties.length;
        for( var i = 0; i < plength; i++ ) {
            if( this.state.properties[i].key === property.key ) {
                index = i;
                break;
            }
        }
        this.state.properties.splice( index, 1 );
        this.setState( {properties: this.state.properties} );
    },

    render() {
        return (
            <div>
                <PropertiesList plist={this.state.properties} onPropertyRemove={this.handleRowRemove}/>
                <NewRow/>
            </div>
        );
    }
});

const PropertiesList = React.createClass({
    handleCompanyRemove(property){
        this.props.onPropertyRemove( property );
    },

    render() {
        var properties = [];
        var that = this;
        console.log(this.props.plist);
        this.props.plist.forEach(function(property) {
            properties.push(<Property property={property} onPropertyDelete={that.handleRowRemove} /> );
        });
        return (
            <div>
                <table className="table table-striped">
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Value</th>
                            <th style={{width: 50}}>Action</th>
                        </tr>
                    </thead>
                    <tbody>{properties}</tbody>
                </table>
            </div>
        );
    }
});

const Property = React.createClass({
    handleRemoveProperty() {
        this.props.onRowDelete( this.props.company );
        return false;
    },

    render: function() {
        return (
            <tr>
                <td>{this.props.property.key}</td>
                <td>{this.props.property.value}</td>
                <td><div className="pull-right">
                        <Button className="btn btn-danger btn-xs" onClick={this.handleRemoveProperty}>Delete</Button>
                    </div>
                </td>
            </tr>
        );
    }
});

const NewRow = React.createClass({
    handleSubmit() {
        var key = ReactDOM.findDOMNode(this.refs.propertykey).value;
        var value = ReactDOM.findDOMNode(this.refs.propertyvalue).value;
        var newrow = {key: key, value: value };
        this.props.onRowSubmit( newrow );

        this.refs.key.getDOMNode().value = '';
        this.refs.value.getDOMNode().value = '';
        return false;
    },

    render() {
        return (
            <form className="form-inline" onSubmit={this.handleSubmit}>
                <fieldset className="form-group">
                    <Input type="text" id="property-key" className="form-control col-md-8"  placeholder="Name" ref="propertykey"/>
                    <Input type="text" id="property-value" className="form-control col-md-8" placeholder="Value" ref="propertyvalue"/>
                    <Button type="submit" className="btn btn-primary">Add Property</Button>
                </fieldset>
            </form>
        );
    }
});

export default PropertiesTable;
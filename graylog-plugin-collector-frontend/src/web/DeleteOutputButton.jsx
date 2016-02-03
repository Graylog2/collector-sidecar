import React from 'react';
import { Button } from 'react-bootstrap';

const DeleteOutputButton = React.createClass({
    propTypes: {
        output: React.PropTypes.object.isRequired,
        onClick: React.PropTypes.func.isRequired,
    },

    handleClick() {
        if (window.confirm('Really delete output?')) {
            this.props.onClick(this.props.output, function () {});
        }
    },

    render() {
        return (
            <Button className="btn btn-danger btn-xs" onClick={this.handleClick}>
                Delete output
            </Button>
        );
    },
});

export default DeleteOutputButton;

import React from 'react';
import { Button } from 'react-bootstrap';

const DeleteInputButton = React.createClass({
    propTypes: {
        input: React.PropTypes.object.isRequired,
        onClick: React.PropTypes.func.isRequired,
    },

    handleClick() {
        if (window.confirm('Really delete input?')) {
            this.props.onClick(this.props.input, function () {});
        }
    },

    render() {
        return (
            <Button className="btn btn-danger btn-xs" onClick={this.handleClick}>
                Delete input
            </Button>
        );
    },
});

export default DeleteInputButton;

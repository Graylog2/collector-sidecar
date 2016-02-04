import React from 'react';
import { Button } from 'react-bootstrap';

const DeleteSnippetButton = React.createClass({
    propTypes: {
        snippet: React.PropTypes.object.isRequired,
        onClick: React.PropTypes.func.isRequired,
    },

    handleClick() {
        if (window.confirm('Really delete snippet?')) {
            this.props.onClick(this.props.snippet, function () {});
        }
    },

    render() {
        return (
            <Button className="btn btn-danger btn-xs" onClick={this.handleClick}>
                Delete snippet
            </Button>
        );
    },
});

export default DeleteSnippetButton;

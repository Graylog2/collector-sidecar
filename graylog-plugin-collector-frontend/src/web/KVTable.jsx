import React, {PropTypes} from 'react';
import { Button, Input } from 'react-bootstrap';

import ObjectUtils from 'util/ObjectUtils';

const KVTable = React.createClass({
  propTypes: {
    pairs: PropTypes.object.isRequired,
    headers: PropTypes.array,
    editable: PropTypes.bool,
    onChange: PropTypes.func,
    className: PropTypes.string,
    tableClassName: PropTypes.string,
    actionsSize: PropTypes.oneOf(['large', 'medium', 'small', 'xsmall']),
  },

  getInitialState() {
    return {
      newKey: '',
      newValue: '',
    };
  },

  getDefaultProps() {
    return {
      headers: ['Name', 'Value', 'Actions'],
      editable: false,
      actionsSize: 'xsmall',
    };
  },

  _onPairsChange(newPairs) {
    if (this.props.onChange) {
      this.props.onChange(newPairs);
    }
  },

  _bindValue(event) {
    const newState = {};
    newState[event.target.name] = event.target.value;
    this.setState(newState);
  },

  _addRow() {
    const newPairs = ObjectUtils.clone(this.props.pairs);
    newPairs[this.state.newKey] = this.state.newValue;
    this._onPairsChange(newPairs);

    this.setState({newKey: '', newValue: ''});
  },

  _deleteRow(key) {
    return () => {
      if (window.confirm(`Are you sure you want to delete property '${key}'?`)) {
        const newPairs = ObjectUtils.clone(this.props.pairs);
        delete newPairs[key];
        this._onPairsChange(newPairs);
      }
    };
  },

  _formattedHeaders(headers) {
    return (
      <tr>
        {headers.map((header, idx) => {
          const style = {};
          // Assign width to last column so it sticks to the right
          if (idx === headers.length - 1) {
            style.width = 75;
          }

          return <th key={header} style={style}>{header}</th>;
        })}
      </tr>
    );
  },

  _formattedRows(pairs) {
    return Object.keys(pairs).sort().map(key => {
      const actions = [];
      if (this.props.editable) {
        actions.push(
          <Button key={`delete-${key}`} bsStyle="danger" bsSize={this.props.actionsSize} onClick={this._deleteRow(key)}>
            Delete
          </Button>
        );
      }

      return (
        <tr key={key}>
          <td>{key}</td>
          <td>{pairs[key]}</td>
          <td>
            {actions}
          </td>
        </tr>
      );
    });
  },

  _newRow() {
    const addRowDisabled = !this.state.newKey || !this.state.newValue;
    return (
      <tr>
        <td>
          <Input type="text" name="newKey" id="newKey" bsSize="small" placeholder="Name" value={this.state.newKey}
                 onChange={this._bindValue}/>
        </td>
        <td>
          <Input type="text" name="newValue" id="newValue" bsSize="small" placeholder="Value"
                 value={this.state.newValue} onChange={this._bindValue}/>
        </td>
        <td>
          <Button bsStyle="success" bsSize="small" onClick={this._addRow} disabled={addRowDisabled}>Add</Button>
        </td>
      </tr>
    );
  },

  render() {
    return (
      <div>
        <div className={`table-responsive ${this.props.className}`}>
          <table className="table table-striped">
            <thead>{this._formattedHeaders(this.props.headers)}</thead>
            <tbody>
            {this._formattedRows(this.props.pairs)}
            {this._newRow()}
            </tbody>
          </table>
        </div>
      </div>
    );
  },
});

export default KVTable;

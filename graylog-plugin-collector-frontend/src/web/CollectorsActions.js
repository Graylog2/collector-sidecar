import Reflux from 'reflux';

const CollectorsActions = Reflux.createActions({
    'list': {asyncResult: true},
    'getConfiguration': {asyncResult: true},
    'saveInput': {asyncResult: true},
    'deleteInput': {asyncResult: true},
    'saveOutput': {asyncResult: true},
    'deleteOutput': {asyncResult: true},
    'saveSnippet': {asyncResult: true},
    'deleteSnippet': {asyncResult: true},
});

export default CollectorsActions;
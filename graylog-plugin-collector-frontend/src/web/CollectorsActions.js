import Reflux from 'reflux';

const CollectorsActions = Reflux.createActions({
    'list': {asyncResult: true},
    'getConfiguration': {asyncResult: true},
});

export default CollectorsActions;
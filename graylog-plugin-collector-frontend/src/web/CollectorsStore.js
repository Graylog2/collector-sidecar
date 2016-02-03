import Reflux from 'reflux';
import URLUtils from 'util/URLUtils';
import UserNotification from 'util/UserNotification';
import fetch from 'logic/rest/FetchProvider';

import CollectorsActions from './CollectorsActions';

const CollectorsStore = Reflux.createStore({
    listenables: [CollectorsActions],
    sourceUrl: '/plugins/org.graylog.plugins.collector',
    collectors: undefined,

    init() {
        this.trigger({collectors: this.collectors});
    },

    list() {
        const promise = fetch('GET', URLUtils.qualifyUrl(this.sourceUrl));
        promise
            .then(
                response => {
                    this.collectors = response.collectors;
                    this.trigger({collectors: this.collectors});

                    return this.collectors;
                },
                error => {
                    UserNotification.error('Fetching Collectors failed with status: ' + error,
                        'Could not retrieve Collectors');
                });
        CollectorsActions.list.promise(promise);
    },

    getConfiguration(collectorId) {
        const promise = fetch('GET', URLUtils.qualifyUrl(this.sourceUrl + '/' + collectorId));
        promise
            .catch(
                error => {
                    UserNotification.error('Fetching collector configuration failed with status: ' + error,
                        'Could not retrieve configuration');
                });
        CollectorsActions.getConfiguration.promise(promise);
    },

    saveInput(input, collectorid) {
        const requestInput = {
            type: 'nxlog',
            name: input.name,
            forward_to: input.forwardto,
            properties: input.properties,
        };

        let url =  URLUtils.qualifyUrl(this.sourceUrl + '/' + collectorid + '/inputs');
        let method;
        if (input.id === "") {
            method = 'POST';
        } else {
            requestInput.input_id = input.id;
            url += '/' + input.id;
            method = 'PUT';
        }

        const promise = fetch(method, url, requestInput);
        promise
            .then(() => {
                    var action = input.id === "" ? "created" : "updated";
                    var message = "Collector input \"" + input.name + "\" successfully " + action;
                    UserNotification.success(message);
                }, (error) => {
                    UserNotification.error("Saving input \"" + input.name + "\" failed with status: " + error.message,
                        "Could not save Input");
                });

        CollectorsActions.saveInput.promise(promise);
    },

    deleteInput(input, collectorid) {
        let url =  URLUtils.qualifyUrl(this.sourceUrl + '/' + collectorid + '/inputs');
        const promise = fetch('DELETE', url + "/" + input.input_id);
        promise
            .then(() => {
                UserNotification.success("Input \"" + input.name + "\" successfully deleted");
            }, (error) => {
                UserNotification.error("Deleting Input \"" + input.name + "\" failed with status: " + error.message,
                    "Could not delete Input");
            });

        CollectorsActions.deleteInput.promise(promise);
    },

    saveOutput(output, collectorid) {
        const requestOutput = {
            type: 'nxlog',
            name: output.name,
            properties: output.properties,
        };

        let url =  URLUtils.qualifyUrl(this.sourceUrl + '/' + collectorid + '/outputs');
        let method;
        if (output.id === "") {
            method = 'POST';
        } else {
            requestOutput.output_id = output.id;
            url += '/' + output.id;
            method = 'PUT';
        }

        const promise = fetch(method, url, requestOutput);
        promise
            .then(() => {
                var action = output.id === "" ? "created" : "updated";
                var message = "Collector output \"" + output.name + "\" successfully " + action;
                UserNotification.success(message);
            }, (error) => {
                UserNotification.error("Saving output \"" + output.name + "\" failed with status: " + error.message,
                    "Could not save Output");
            });

        CollectorsActions.saveOutput.promise(promise);
    },

    deleteOutput(output, collectorid) {
        let url =  URLUtils.qualifyUrl(this.sourceUrl + '/' + collectorid + '/outputs');
        const promise = fetch('DELETE', url + "/" + output.output_id);
        promise
            .then(() => {
                UserNotification.success("Output \"" + output.name + "\" successfully deleted");
            }, (error) => {
                UserNotification.error("Deleting Output \"" + output.name + "\" failed with status: " + error.message,
                    "Could not delete Output");
            });

        CollectorsActions.deleteOutput.promise(promise);
    },

});

export default CollectorsStore;

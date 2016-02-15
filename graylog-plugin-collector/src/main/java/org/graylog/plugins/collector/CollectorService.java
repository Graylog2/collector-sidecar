package org.graylog.plugins.collector;


import com.google.common.collect.Iterables;
import com.mongodb.BasicDBObject;
import org.bson.types.ObjectId;
import org.graylog.plugins.collector.rest.models.CollectorConfiguration;
import org.graylog.plugins.collector.rest.models.CollectorConfigurationSnippet;
import org.graylog.plugins.collector.rest.models.CollectorInput;
import org.graylog.plugins.collector.rest.models.CollectorOutput;
import org.graylog2.bindings.providers.MongoJackObjectMapperProvider;
import org.graylog2.database.MongoConnection;
import org.mongojack.DBCursor;
import org.mongojack.DBQuery;
import org.mongojack.JacksonDBCollection;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.inject.Inject;
import javax.inject.Singleton;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;

@Singleton
public class CollectorService {
    private static final String COLLECTION_NAME = "collector_configurations";
    private static final Logger log = LoggerFactory.getLogger(CollectorService.class);

    private final JacksonDBCollection<CollectorConfiguration, ObjectId> dbCollection;

    @Inject
    public CollectorService(MongoConnection mongoConnection,
                            MongoJackObjectMapperProvider mapper) {
        dbCollection = JacksonDBCollection.wrap(
                mongoConnection.getDatabase().getCollection(COLLECTION_NAME),
                CollectorConfiguration.class,
                ObjectId.class,
                mapper.get());
    }

    public List<CollectorConfiguration> loadAll() {
        final DBCursor<CollectorConfiguration> cursor = dbCollection.find();
        final List<CollectorConfiguration> collectorConfigurationList = new ArrayList<>();
        Iterables.addAll(collectorConfigurationList, cursor);
       return collectorConfigurationList;
    }

    public CollectorConfiguration findByCollectorId(String collectorId) {
        return dbCollection.findOne(DBQuery.is("collector_id", collectorId));
    }

    public List<CollectorConfiguration> findByTags(List tags) {
        final DBCursor<CollectorConfiguration> cursor = dbCollection.find(DBQuery.all("tags", tags));

        final List<CollectorConfiguration> result = new ArrayList<>();
        while (cursor.hasNext()) {
            result.add(cursor.next());
        }
        return result;
    }

    public CollectorConfiguration save(CollectorConfiguration configuration) {
        return dbCollection.findAndModify(DBQuery.is("collector_id", configuration.collectorId()), new BasicDBObject(),
                new BasicDBObject(), false, configuration, true, true);
    }

    public int delete(String collectorId) {
        return dbCollection.remove(DBQuery.is("collector_id", collectorId)).getN();
    }

    public CollectorConfiguration merge(List<CollectorConfiguration> configurations) {
        CollectorConfiguration result;
        final Iterator<CollectorConfiguration> cursor = configurations.iterator();
        if (cursor.hasNext()) {
            result = cursor.next();
            result.tags().clear();
            while (cursor.hasNext()) {
                result.mergeWith(cursor.next());
            }
            return result;
        }

        return null;
    }

    public int deleteInput(String collectorId, String inputId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        List<CollectorInput> inputList = collectorConfiguration.inputs();
        int deleted = 0;
        if (inputList != null) {
            for (int i = 0; i < inputList.size(); i++) {
                CollectorInput input = inputList.get(i);
                if (input.inputId().equals(inputId)) {
                    collectorConfiguration.inputs().remove(i);
                    deleted++;
                }
            }
            save(collectorConfiguration);
        }
        return deleted;
    }

    public int deleteOutput(String collectorId, String outputId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        List<CollectorOutput> outputList = collectorConfiguration.outputs();
        int deleted = 0;
        if (outputList != null) {
            for (int i = 0; i < outputList.size(); i++) {
                CollectorOutput output = outputList.get(i);
                if (output.outputId().equals(outputId)) {
                    collectorConfiguration.outputs().remove(i);
                    deleted++;
                }
            }
            save(collectorConfiguration);
        }
        return deleted;
    }

    public int deleteSnippet(String collectorId, String snippetId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        List<CollectorConfigurationSnippet> snippetList = collectorConfiguration.snippets();
        int deleted = 0;
        if (snippetList != null) {
            for (int i = 0; i < snippetList.size(); i++) {
                CollectorConfigurationSnippet snippet = snippetList.get(i);
                if (snippet.snippetId().equals(snippetId)) {
                    collectorConfiguration.snippets().remove(i);
                    deleted++;
                }
            }
            save(collectorConfiguration);
        }
        return deleted;
    }

    public List<CollectorInput> loadAllInputs(String collectorId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        return collectorConfiguration.inputs();
    }

    public List<CollectorOutput> loadAllOutputs(String collectorId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        return collectorConfiguration.outputs();
    }

    public List<CollectorConfigurationSnippet> loadAllSnippets(String collectorId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        return collectorConfiguration.snippets();
    }

    public CollectorConfiguration withInputFromRequest(String collectorId, CollectorInput input) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        collectorConfiguration.inputs().add(input);
        return collectorConfiguration;
    }

    public CollectorConfiguration withOutputFromRequest(String collectorId, CollectorOutput output) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        collectorConfiguration.outputs().add(output);
        return collectorConfiguration;
    }

    public CollectorConfiguration withSnippetFromRequest(String collectorId, CollectorConfigurationSnippet snippet) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        collectorConfiguration.snippets().add(snippet);
        return collectorConfiguration;
    }

    public CollectorConfiguration withTagsFromRequest(String id, List<String> tags) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("_id", id));
        collectorConfiguration.tags().clear();
        collectorConfiguration.tags().addAll(tags);
        return collectorConfiguration;
    }
}

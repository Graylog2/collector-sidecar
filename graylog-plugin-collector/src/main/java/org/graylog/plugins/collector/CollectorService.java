package org.graylog.plugins.collector;


import com.mongodb.BasicDBObject;
import org.bson.types.ObjectId;
import org.graylog.plugins.collector.rest.models.CollectorConfiguration;
import org.graylog.plugins.collector.rest.models.CollectorInput;
import org.graylog.plugins.collector.rest.models.CollectorOutput;
import org.graylog2.bindings.providers.MongoJackObjectMapperProvider;
import org.graylog2.collectors.CollectorServiceImpl;
import org.graylog2.database.MongoConnection;
import org.mongojack.DBQuery;
import org.mongojack.JacksonDBCollection;

import javax.inject.Inject;
import javax.inject.Singleton;
import java.util.ArrayList;
import java.util.List;

@Singleton
public class CollectorService {
    private static final String COLLECTION_NAME = "collector_configurations";

    private final JacksonDBCollection<CollectorConfiguration, ObjectId> dbCollection;
    private final CollectorServiceImpl serverCollectorService;

    @Inject
    public CollectorService(MongoConnection mongoConnection,
                            MongoJackObjectMapperProvider mapper,
                            CollectorServiceImpl serverCollectorService) {
        this.serverCollectorService = serverCollectorService;
        dbCollection = JacksonDBCollection.wrap(
                mongoConnection.getDatabase().getCollection(COLLECTION_NAME),
                CollectorConfiguration.class,
                ObjectId.class,
                mapper.get());
    }

    public CollectorConfiguration createIfRegistered(String collectorId) {
        if (serverCollectorService.findById(collectorId) != null) {
            CollectorConfiguration newConfiguration = CollectorConfiguration.create(collectorId,
                    new ArrayList<CollectorInput>(), new ArrayList<CollectorOutput>());
            save(newConfiguration);
            return newConfiguration;
        }
        return null;
    }

    public CollectorConfiguration findById(String collectorId) {
        return dbCollection.findOne(DBQuery.is("collector_id", collectorId));
    }

    public CollectorConfiguration save(CollectorConfiguration configuration) {
        return dbCollection.findAndModify(DBQuery.is("collector_id", configuration.collectorId()), new BasicDBObject(),
                new BasicDBObject(), false, configuration, true, true);
    }

    public int delete(String collectorId) {
        return dbCollection.remove(DBQuery.is("collector_id", collectorId)).getN();
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

    public List<CollectorInput> loadAllInputs(String collectorId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        return collectorConfiguration.inputs();
    }

    public List<CollectorOutput> loadAllOutputs(String collectorId) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        return collectorConfiguration.outputs();
    }

    public CollectorConfiguration withInputFromRequest(String collectorId, CollectorInput input) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        if (collectorConfiguration != null) {
            collectorConfiguration.inputs().add(input);
        }
        return collectorConfiguration;
    }

    public CollectorConfiguration withOutputFromRequest(String collectorId, CollectorOutput output) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("collector_id", collectorId));
        if (collectorConfiguration != null) {
            collectorConfiguration.outputs().add(output);
        }
        return collectorConfiguration;
    }
}

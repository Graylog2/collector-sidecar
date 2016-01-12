package org.graylog.plugins.collector;


import org.bson.types.ObjectId;
import org.graylog2.bindings.providers.MongoJackObjectMapperProvider;
import org.graylog2.database.MongoConnection;
import org.mongojack.DBQuery;
import org.mongojack.JacksonDBCollection;
import org.mongojack.WriteResult;

import javax.inject.Inject;
import javax.inject.Singleton;
import java.util.List;

@Singleton
public class CollectorService {
    private static final String COLLECTION_NAME = "collector_configurations";

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

    public CollectorConfiguration findById(String id) {
        return dbCollection.findOne(DBQuery.is("id", id));
    }

    public CollectorConfiguration save(CollectorConfiguration configuration) {
        final WriteResult<CollectorConfiguration, ObjectId> result = dbCollection.save(configuration);
        return result.getSavedObject();
    }

    public int delete(String configurationId) {
        return dbCollection.remove(DBQuery.is("id", configurationId)).getN();
    }

    public List<CollectorInput> loadAllInputs(String id) {
        CollectorConfiguration collectorConfiguration = dbCollection.findOne(DBQuery.is("id", id));
        return collectorConfiguration.inputs();
    }
}

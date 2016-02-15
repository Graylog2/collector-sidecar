package org.graylog.plugins.collector.rest.models;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.mongojack.ObjectId;

import java.util.List;

@AutoValue
public abstract class CollectorConfigurationSummary {
    @JsonProperty("_id")
    @ObjectId
    public abstract String getId();

    @JsonProperty("tags")
    public abstract List<String> tags();

    @JsonCreator
    public static CollectorConfigurationSummary create(@JsonProperty("_id") String id,
                                                @JsonProperty("tags") List<String> tags) {
        return new AutoValue_CollectorConfigurationSummary(id, tags);
    }

}


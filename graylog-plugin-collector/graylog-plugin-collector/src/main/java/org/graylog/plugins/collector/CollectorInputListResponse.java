package org.graylog.plugins.collector;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;

import java.util.Collection;

@AutoValue
public abstract class CollectorInputListResponse {
    @JsonProperty
    public abstract long total();

    @JsonProperty
    public abstract Collection<CollectorInput> inputs();

    @JsonCreator
    public static CollectorInputListResponse create(@JsonProperty("total") long total,
                                                    @JsonProperty("inputs") Collection<CollectorInput> inputs) {
        return new AutoValue_CollectorInputListResponse(total, inputs);
    }
}

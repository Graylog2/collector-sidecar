package org.graylog.plugins.collector.rest.responses;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.graylog.plugins.collector.rest.models.CollectorOutput;

import java.util.Collection;

@AutoValue
public abstract class CollectorOutputListResponse {
    @JsonProperty
    public abstract long total();

    @JsonProperty
    public abstract Collection<CollectorOutput> outputs();

    @JsonCreator
    public static CollectorOutputListResponse create(@JsonProperty("total") long total,
                                                     @JsonProperty("outputs") Collection<CollectorOutput> outputs) {
        return new AutoValue_CollectorOutputListResponse(total, outputs);
    }
}

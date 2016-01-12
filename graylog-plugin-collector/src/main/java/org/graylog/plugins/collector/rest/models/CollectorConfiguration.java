package org.graylog.plugins.collector.rest.models;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.hibernate.validator.constraints.NotEmpty;

import javax.annotation.Nullable;
import java.util.List;


@AutoValue
public abstract class CollectorConfiguration {
    @JsonProperty("collector_id")
    public abstract String collectorId();

    @JsonProperty
    @Nullable
    public abstract List<CollectorInput> inputs();

    @JsonProperty
    @Nullable
    public abstract List<CollectorOutput> outputs();

    @JsonCreator
    public static CollectorConfiguration create(@JsonProperty("_id") String objectId,
                                                @JsonProperty("collector_id") String collectorId,
                                                @JsonProperty("inputs") @Nullable List<CollectorInput> inputs,
                                                @JsonProperty("outputs") @Nullable List<CollectorOutput> outputs) {
        return new AutoValue_CollectorConfiguration(collectorId, inputs, outputs);
    }

    public static CollectorConfiguration create(@NotEmpty String collectorId,
                                                @NotEmpty List<CollectorInput> inputs,
                                                @NotEmpty List<CollectorOutput> outputs) {
        return create(null, collectorId, inputs, outputs);
    }
}

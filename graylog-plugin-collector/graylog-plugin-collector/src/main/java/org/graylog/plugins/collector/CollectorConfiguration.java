package org.graylog.plugins.collector;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.hibernate.validator.constraints.NotEmpty;
import org.mongojack.Id;
import org.mongojack.ObjectId;

import javax.annotation.Nullable;
import java.util.List;


@AutoValue
public abstract class CollectorConfiguration {
    @JsonProperty("id")
    public abstract String id();

    @JsonProperty
    @Nullable
    public abstract List<CollectorInput> inputs();

    @JsonProperty
    @Nullable
    public abstract List<CollectorOutput> outputs();

    @JsonCreator
    public static CollectorConfiguration create(@JsonProperty("_id") String objectId,
                                                @JsonProperty("id") String id,
                                                @JsonProperty("inputs") @Nullable List<CollectorInput> inputs,
                                                @JsonProperty("outputs") @Nullable List<CollectorOutput> outputs) {
        return new AutoValue_CollectorConfiguration(id, inputs, outputs);
    }

    public static CollectorConfiguration create(@NotEmpty String id,
                                                @NotEmpty List<CollectorInput> inputs,
                                                @NotEmpty List<CollectorOutput> outputs) {
        return create(null, id, inputs, outputs);
    }
}

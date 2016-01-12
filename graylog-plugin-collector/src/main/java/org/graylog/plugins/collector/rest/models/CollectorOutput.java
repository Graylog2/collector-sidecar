package org.graylog.plugins.collector.rest.models;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.hibernate.validator.constraints.NotEmpty;
import org.mongojack.ObjectId;

import javax.annotation.Nullable;
import java.util.Map;

@AutoValue
public abstract class CollectorOutput {
    @JsonProperty("output_id")
    @ObjectId
    public abstract String outputId();

    @JsonProperty
    public abstract String type();

    @JsonProperty
    public abstract String name();

    @JsonProperty
    @Nullable
    public abstract Map<String, Object> properties();

    @JsonCreator
    public static CollectorOutput create(@JsonProperty("output_id") String outputId,
                                         @JsonProperty("type") String type,
                                         @JsonProperty("name") String name,
                                         @JsonProperty("properties") Map<String, Object> properties) {
        if (outputId == null) {
            outputId = org.bson.types.ObjectId.get().toString();
        }
        return new AutoValue_CollectorOutput(outputId, type, name, properties);
    }

    public static CollectorOutput create(@NotEmpty String type,
                                         @NotEmpty String name,
                                         @NotEmpty Map<String, Object> properties) {
        return create(org.bson.types.ObjectId.get().toString(), type, name, properties);
    }

}

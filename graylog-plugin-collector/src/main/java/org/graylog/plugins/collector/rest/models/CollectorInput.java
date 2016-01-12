package org.graylog.plugins.collector.rest.models;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.hibernate.validator.constraints.NotEmpty;
import org.mongojack.ObjectId;

import javax.annotation.Nullable;
import java.util.Map;

@AutoValue
public abstract class CollectorInput {
    @JsonProperty("input_id")
    @ObjectId
    public abstract String inputId();

    @JsonProperty
    public abstract String type();

    @JsonProperty
    public abstract String name();

    @JsonProperty
    public abstract String forwardTo();

    @JsonProperty
    @Nullable
    public abstract Map<String, Object> properties();

    @JsonCreator
    public static CollectorInput create(@JsonProperty("input_id") String inputId,
                                        @JsonProperty("type") String type,
                                        @JsonProperty("name") String name,
                                        @JsonProperty("forward_to") String forwardTo,
                                        @JsonProperty("properties") Map<String, Object> properties) {
        if (inputId == null) {
            inputId = org.bson.types.ObjectId.get().toString();
        }
        return new AutoValue_CollectorInput(inputId, type, name, forwardTo, properties);
    }

    public static CollectorInput create(@NotEmpty String type,
                                        @NotEmpty String name,
                                        @NotEmpty String forwardTo,
                                        @NotEmpty Map<String, Object> properties) {
        return create(org.bson.types.ObjectId.get().toString(), type, name, forwardTo, properties);
    }
}

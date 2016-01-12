package org.graylog.plugins.collector;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;

import javax.annotation.Nullable;
import java.util.Map;

@AutoValue
public abstract class CollectorInput {
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
    public static CollectorInput create(@JsonProperty("type") String type,
                                        @JsonProperty("name") String name,
                                        @JsonProperty("forward_to") String forwardTo,
                                        @JsonProperty("properties") Map<String, Object> properties) {
        return new AutoValue_CollectorInput(type, name, forwardTo, properties);
    }
}

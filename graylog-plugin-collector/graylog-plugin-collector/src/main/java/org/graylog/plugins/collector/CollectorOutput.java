package org.graylog.plugins.collector;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;

import javax.annotation.Nullable;
import java.util.Map;

@AutoValue
public abstract class CollectorOutput {
    @JsonProperty
    public abstract String type();

    @JsonProperty
    public abstract String name();

    @JsonProperty
    @Nullable
    public abstract Map<String, Object> properties();

    @JsonCreator
    public static CollectorOutput create(@JsonProperty("type") String type,
                                        @JsonProperty("name") String name,
                                        @JsonProperty("properties") Map<String, Object> properties) {
        return new AutoValue_CollectorOutput(type, name, properties);
    }
}

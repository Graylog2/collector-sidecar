package org.graylog.plugins.collector.rest.models;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.hibernate.validator.constraints.NotEmpty;
import org.mongojack.ObjectId;

import java.util.List;


@AutoValue
public abstract class CollectorConfiguration {
    @JsonProperty("_id")
    @ObjectId
    public abstract String getId();

    @JsonProperty("collector_id")
    public abstract String collectorId();

    @JsonProperty("tags")
    public abstract List<String> tags();

    @JsonProperty
    public abstract List<CollectorInput> inputs();

    @JsonProperty
    public abstract List<CollectorOutput> outputs();

    @JsonProperty
    public abstract List<CollectorConfigurationSnippet> snippets();

    @JsonCreator
    public static CollectorConfiguration create(@JsonProperty("_id") String id,
                                                @JsonProperty("collector_id") String collectorId,
                                                @JsonProperty("tags") List<String> tags,
                                                @JsonProperty("inputs") List<CollectorInput> inputs,
                                                @JsonProperty("outputs") List<CollectorOutput> outputs,
                                                @JsonProperty("snippets") List<CollectorConfigurationSnippet> snippets) {
        return new AutoValue_CollectorConfiguration(id, collectorId, tags, inputs, outputs, snippets);
    }

    public static CollectorConfiguration create(@NotEmpty String collectorId,
                                                @NotEmpty List<String> tags,
                                                @NotEmpty List<CollectorInput> inputs,
                                                @NotEmpty List<CollectorOutput> outputs,
                                                @NotEmpty List<CollectorConfigurationSnippet> snippets) {
        return create(null, collectorId, tags, inputs, outputs, snippets);
    }

    public void mergeWith(CollectorConfiguration collectorConfiguration) {
        if (collectorConfiguration.inputs() != null) this.inputs().addAll(collectorConfiguration.inputs());
        if (collectorConfiguration.outputs() != null) this.outputs().addAll(collectorConfiguration.outputs());
        if (collectorConfiguration.snippets() != null) this.snippets().addAll(collectorConfiguration.snippets());
    }
}

package org.graylog.plugins.collector.rest.responses;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.auto.value.AutoValue;
import org.graylog.plugins.collector.rest.models.CollectorConfigurationSnippet;

import java.util.Collection;

@AutoValue
public abstract class CollectorSnippetListResponse {
    @JsonProperty
    public abstract long total();

    @JsonProperty
    public abstract Collection<CollectorConfigurationSnippet> snippets();

    @JsonCreator
    public static CollectorSnippetListResponse create(@JsonProperty("total") long total,
                                                      @JsonProperty("snippets") Collection<CollectorConfigurationSnippet> snippets) {
        return new AutoValue_CollectorSnippetListResponse(total, snippets);
    }
}

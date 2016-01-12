package org.graylog.plugins.collector;

import org.graylog2.plugin.PluginMetaData;
import org.graylog2.plugin.ServerStatus;
import org.graylog2.plugin.Version;

import java.net.URI;
import java.util.EnumSet;
import java.util.Set;

/**
 * Implement the PluginMetaData interface here.
 */
public class CollectorMetaData implements PluginMetaData {
    @Override
    public String getUniqueId() {
        return "org.graylog.plugins.collector.CollectorPlugin";
    }

    @Override
    public String getName() {
        return "Collector";
    }

    @Override
    public String getAuthor() {
        return "Marius Sturm";
    }

    @Override
    public URI getURL() {
        return URI.create("https://www.graylog.org/");
    }

    @Override
    public Version getVersion() {
        return new Version(1, 0, 0);
    }

    @Override
    public String getDescription() {
        return "Collector configuration plugin";
    }

    @Override
    public Version getRequiredVersion() {
        return new Version(1, 3, 0);
    }

    @Override
    public Set<ServerStatus.Capability> getRequiredCapabilities() {
        return EnumSet.of(ServerStatus.Capability.SERVER);
    }
}

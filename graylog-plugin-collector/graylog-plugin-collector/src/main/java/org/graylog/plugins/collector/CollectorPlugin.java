package org.graylog.plugins.collector;

import org.graylog2.plugin.Plugin;
import org.graylog2.plugin.PluginMetaData;
import org.graylog2.plugin.PluginModule;

import java.util.Collection;
import java.util.Collections;

/**
 * Implement the Plugin interface here.
 */
public class CollectorPlugin implements Plugin {
    @Override
    public PluginMetaData metadata() {
        return new CollectorMetaData();
    }

    @Override
    public Collection<PluginModule> modules () {
        return Collections.<PluginModule>singleton(new CollectorModule());
    }
}

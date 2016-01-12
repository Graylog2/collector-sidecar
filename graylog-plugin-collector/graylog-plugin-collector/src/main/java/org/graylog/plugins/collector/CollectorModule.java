package org.graylog.plugins.collector;

import org.graylog2.plugin.PluginModule;

public class CollectorModule extends PluginModule {
    @Override
    protected void configure() {
        bind(CollectorService.class).asEagerSingleton();

        addRestResource(CollectorConfigurationResource.class);

        addConfigBeans();
    }
}

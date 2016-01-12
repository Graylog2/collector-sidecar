package org.graylog.plugins.collector;


import com.wordnik.swagger.annotations.*;
import org.graylog2.collectors.Collector;
import org.graylog2.collectors.CollectorServiceImpl;
import org.graylog2.database.NotFoundException;
import org.graylog2.plugin.rest.PluginRestResource;
import org.graylog2.shared.rest.resources.RestResource;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.inject.Inject;
import javax.mail.internet.AddressException;
import javax.ws.rs.*;
import javax.ws.rs.core.MediaType;
import java.net.*;
import java.util.*;

@Api(value = "Collector configuration", description = "Manage collector configurations")
@Path("/")
public class CollectorConfigurationResource extends RestResource implements PluginRestResource {
    private static final Logger log = LoggerFactory.getLogger(CollectorConfigurationResource.class);

    private final CollectorService collectorService;
    private final CollectorServiceImpl serverCollectorService;

    @Inject
    public CollectorConfigurationResource(CollectorService collectorService,
                                          CollectorServiceImpl serverCollectorService) {
        this.collectorService = collectorService;
        this.serverCollectorService = serverCollectorService;
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    @ApiOperation(value = "List all collectors")
    public List<Collector> listCollectors() {
        return serverCollectorService.all();
    }

    @GET
    @Path("/{collectorId}")
    @Produces(MediaType.APPLICATION_JSON)
    @ApiOperation(value = "Get single collector configuration")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public CollectorConfiguration getConfiguration(@ApiParam(name = "collectorId",
            required = true) @PathParam("collectorId") String collectorId) throws NotFoundException {

        return collectorService.findById(collectorId);
    }

    @DELETE
    @Path("/{collectorId}")
    @ApiOperation(value = "Delete a collector configuration")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public void deleteConfiguration(@ApiParam(name = "collectorId",
            required = true) @PathParam("collectorId") String collectorId) throws NotFoundException {
        collectorService.delete(collectorId);
    }

    @GET
    @Path("/{collectorId}/inputs")
    @Produces(MediaType.APPLICATION_JSON)
    @ApiOperation(value = "List collector inputs")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public CollectorInputListResponse getInputs(@ApiParam(name = "collectorId",
            required = true) @PathParam("collectorId") String collectorId) throws NotFoundException {
        final List<CollectorInput> collectorInputs = collectorService.loadAllInputs(collectorId);
        return CollectorInputListResponse.create(collectorInputs.size(), collectorInputs);
    }

    @GET
    @Path("/configuration/{collectorId}/new")
    @Produces(MediaType.APPLICATION_JSON)
    @ApiOperation(value = "Create new collector configuration")
    public CollectorConfiguration newConfiguration(@ApiParam(name = "collectorId",
            required = true) @PathParam("collectorId") String collectorId) {
        String ip;
        try {
            InetAddress inetAddr = getLocalAddress();
            if (inetAddr != null) {
                ip = inetAddr.toString().replace("/", "");
            } else {
                throw new AddressException();
            }
        } catch (SocketException e) {
            log.warn("Can not get address for eth0");
            return null;
        } catch (AddressException e) {
            log.warn("Can not get address for eth0");
            return null;
        }

        List<CollectorInput> collectorInputs = new ArrayList<>();
        List<CollectorOutput> collectorOutputs = new ArrayList<>();

        HashMap<String, Object> inputProperties = new HashMap<>();
        inputProperties.put("Module", "im_msvistalog");
        collectorInputs.add(CollectorInput.create("nxlog", "windows-eventlog", "gelf-udp", inputProperties));

        HashMap<String, Object> outputProperties = new HashMap<>();
        outputProperties.put("Module", "om_udp");
        outputProperties.put("Host", ip);
        outputProperties.put("Port", "12201");
        outputProperties.put("OutputType", "GELF");
        collectorOutputs.add(CollectorOutput.create("nxlog", "gelf-udp", outputProperties));

        CollectorConfiguration newConfiguration = CollectorConfiguration.create(collectorId, collectorInputs, collectorOutputs);
        collectorService.save(newConfiguration);

        return newConfiguration;
    }

    private static InetAddress getLocalAddress() throws SocketException {
        Enumeration<NetworkInterface> ifaces = NetworkInterface.getNetworkInterfaces();
        while( ifaces.hasMoreElements() )
        {
          NetworkInterface iface = ifaces.nextElement();
          Enumeration<InetAddress> addresses = iface.getInetAddresses();

          while( addresses.hasMoreElements() )
          {
            InetAddress addr = addresses.nextElement();
            if( addr instanceof Inet4Address && !addr.isLoopbackAddress() )
            {
              return addr;
            }
          }
        }

        return null;
  }
}

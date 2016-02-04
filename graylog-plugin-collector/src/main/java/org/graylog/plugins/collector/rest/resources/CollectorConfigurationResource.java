package org.graylog.plugins.collector.rest.resources;


import com.github.joschi.jadconfig.util.Duration;
import com.google.common.base.Function;
import com.google.common.primitives.Ints;
import com.wordnik.swagger.annotations.*;
import org.graylog.plugins.collector.CollectorService;
import org.graylog.plugins.collector.rest.models.CollectorConfiguration;
import org.graylog.plugins.collector.rest.models.CollectorConfigurationSnippet;
import org.graylog.plugins.collector.rest.models.CollectorInput;
import org.graylog.plugins.collector.rest.models.CollectorOutput;
import org.graylog.plugins.collector.rest.responses.CollectorInputListResponse;
import org.graylog.plugins.collector.rest.responses.CollectorOutputListResponse;
import org.graylog.plugins.collector.rest.responses.CollectorSnippetListResponse;
import org.graylog2.collectors.Collector;
import org.graylog2.collectors.CollectorServiceImpl;
import org.graylog2.collectors.Collectors;
import org.graylog2.database.NotFoundException;
import org.graylog2.plugin.rest.PluginRestResource;
import org.graylog2.rest.models.collector.responses.CollectorList;
import org.graylog2.rest.models.collector.responses.CollectorSummary;
import org.graylog2.shared.rest.resources.RestResource;
import org.hibernate.validator.constraints.NotEmpty;
import org.joda.time.DateTime;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.inject.Inject;
import javax.inject.Named;
import javax.mail.internet.AddressException;
import javax.validation.Valid;
import javax.validation.constraints.NotNull;
import javax.ws.rs.*;
import javax.ws.rs.core.MediaType;
import javax.ws.rs.core.Response;
import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.NetworkInterface;
import java.net.SocketException;
import java.util.ArrayList;
import java.util.Enumeration;
import java.util.HashMap;
import java.util.List;

@Api(value = "Collector configuration", description = "Manage collector configurations")
@Path("/")
@Consumes(MediaType.APPLICATION_JSON)
@Produces(MediaType.APPLICATION_JSON)
public class CollectorConfigurationResource extends RestResource implements PluginRestResource {
    private static final Logger log = LoggerFactory.getLogger(CollectorConfigurationResource.class);
    private final LostCollectorFunction lostCollectorFunction;


    private final CollectorService collectorService;
    private final CollectorServiceImpl serverCollectorService;

    @Inject
    public CollectorConfigurationResource(CollectorService collectorService,
                                          CollectorServiceImpl serverCollectorService,
                                          @Named("collector_inactive_threshold") Duration inactiveThreshold) {
        this.collectorService = collectorService;
        this.serverCollectorService = serverCollectorService;
        this.lostCollectorFunction = new LostCollectorFunction(inactiveThreshold.toSeconds());
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    @ApiOperation(value = "List all collectors")
    public CollectorList listCollectors() {
        final List<Collector> collectors = serverCollectorService.all();
        final List<CollectorSummary> collectorSummaries = Collectors.toSummaryList(collectors, lostCollectorFunction);
        return CollectorList.create(collectorSummaries);
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

    @POST
    @Path("/{collectorId}/inputs")
    @ApiOperation(value = "Create a collector input",
            notes = "This is a stateless method which inserts a collector input")
    @ApiResponses(value = {
            @ApiResponse(code = 400, message = "The supplied request is not valid.")
    })
    public Response createInput(@ApiParam(name = "collectorId", required = true)
                             @PathParam("collectorId") @NotEmpty String collectorId,
                             @ApiParam(name = "JSON body", required = true)
                             @Valid @NotNull CollectorInput request) {
        final CollectorConfiguration collectorConfiguration = collectorService.withInputFromRequest(collectorId, request);
        collectorService.save(collectorConfiguration);

        return Response.accepted().build();
    }

    @DELETE
    @Path("/{collectorId}/inputs/{inputId}")
    @ApiOperation(value = "Delete a collector input")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector or Input not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public void deleteInput(@ApiParam(name = "collectorId", required = true)
                                @PathParam("collectorId") String collectorId,
                            @PathParam("inputId") String inputId) throws NotFoundException {
        collectorService.deleteInput(collectorId, inputId);
    }

    @GET
    @Path("/{collectorId}/outputs")
    @Produces(MediaType.APPLICATION_JSON)
    @ApiOperation(value = "List collector outputs")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public CollectorOutputListResponse getOutputs(@ApiParam(name = "collectorId",
            required = true) @PathParam("collectorId") String collectorId) throws NotFoundException {
        final List<CollectorOutput> collectorOutputs = collectorService.loadAllOutputs(collectorId);
        return CollectorOutputListResponse.create(collectorOutputs.size(), collectorOutputs);
    }

    @POST
    @Path("/{collectorId}/outputs")
    @ApiOperation(value = "Create a collector output",
            notes = "This is a stateless method which inserts a collector output")
    @ApiResponses(value = {
            @ApiResponse(code = 400, message = "The supplied request is not valid.")
    })
    public Response createOutput(@ApiParam(name = "collectorId", required = true)
                                    @PathParam("collectorId") @NotEmpty String collectorId,
                                @ApiParam(name = "JSON body", required = true)
                                @Valid @NotNull CollectorOutput request) {
        final CollectorConfiguration collectorConfiguration = collectorService.withOutputFromRequest(collectorId, request);
        collectorService.save(collectorConfiguration);

        return Response.accepted().build();
    }

    @DELETE
    @Path("/{collectorId}/outputs/{outputId}")
    @ApiOperation(value = "Delete a collector output")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector or Output not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public void deleteOutput(@ApiParam(name = "collectorId", required = true)
                            @PathParam("collectorId") String collectorId,
                            @PathParam("outputId") String outputId) throws NotFoundException {
        collectorService.deleteOutput(collectorId, outputId);
    }

    @GET
    @Path("/{collectorId}/snippets")
    @Produces(MediaType.APPLICATION_JSON)
    @ApiOperation(value = "List collector configuration snippets")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public CollectorSnippetListResponse getSnippets(@ApiParam(name = "collectorId",
            required = true) @PathParam("collectorId") String collectorId) throws NotFoundException {
        final List<CollectorConfigurationSnippet> collectorSnippets = collectorService.loadAllSnippets(collectorId);
        return CollectorSnippetListResponse.create(collectorSnippets.size(), collectorSnippets);
    }

    @POST
    @Path("/{collectorId}/snippets")
    @ApiOperation(value = "Create a collector configuration snippet",
            notes = "This is a stateless method which inserts a collector configuration snippet")
    @ApiResponses(value = {
            @ApiResponse(code = 400, message = "The supplied request is not valid.")
    })
    public Response createSnippet(@ApiParam(name = "collectorId", required = true)
                                @PathParam("collectorId") @NotEmpty String collectorId,
                                @ApiParam(name = "JSON body", required = true)
                                @Valid @NotNull CollectorConfigurationSnippet request) {
        final CollectorConfiguration collectorConfiguration = collectorService.withSnippetFromRequest(collectorId, request);
        collectorService.save(collectorConfiguration);

        return Response.accepted().build();
    }

    @DELETE
    @Path("/{collectorId}/snippets/{snippetId}")
    @ApiOperation(value = "Delete a collector configuration snippet")
    @ApiResponses(value = {
            @ApiResponse(code = 404, message = "Collector or Snippet not found."),
            @ApiResponse(code = 400, message = "Invalid ObjectId.")
    })
    public void deleteSnippet(@ApiParam(name = "collectorId", required = true)
                            @PathParam("collectorId") String collectorId,
                            @PathParam("snippetId") String snippetId) throws NotFoundException {
        collectorService.deleteSnippet(collectorId, snippetId);
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
        List<CollectorConfigurationSnippet> collectorConfigurationSnippets = new ArrayList<>();

        HashMap<String, Object> inputProperties = new HashMap<>();
        inputProperties.put("Module", "im_msvistalog");
        collectorInputs.add(CollectorInput.create("nxlog", "windows-eventlog", "gelf-udp", inputProperties));

        HashMap<String, Object> outputProperties = new HashMap<>();
        outputProperties.put("Module", "om_udp");
        outputProperties.put("Host", ip);
        outputProperties.put("Port", "12201");
        outputProperties.put("OutputType", "GELF");
        collectorOutputs.add(CollectorOutput.create("nxlog", "gelf-udp", outputProperties));

        CollectorConfiguration newConfiguration = CollectorConfiguration.create(collectorId, collectorInputs,
                collectorOutputs, collectorConfigurationSnippets);
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

    protected static class LostCollectorFunction implements Function<Collector, Boolean> {
        private final long timeOutInSeconds;

        @Inject
        public LostCollectorFunction(long timeOutInSeconds) {
            this.timeOutInSeconds = timeOutInSeconds;
        }

        @Override
        public Boolean apply(Collector collector) {
            final DateTime threshold = DateTime.now().minusSeconds(Ints.saturatedCast(timeOutInSeconds));
            return collector.getLastSeen().isAfter(threshold);
        }
    }
}

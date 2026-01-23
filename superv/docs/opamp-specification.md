<!--- Hugo front matter used to generate the website version of this page:
title: Open Agent Management Protocol
linkTitle: OpAMP
body_class: otel-docs-spec
github_repo: &repo https://github.com/open-telemetry/opamp-spec
github_project_repo: *repo
path_base_for_github_subdir:
  from: tmp/opamp/index.md
  to: specification.md
cSpell:ignore: bitmask Flipr Fluentd oneof protoc Rpbjpvc varint
--->

# Open Agent Management Protocol

Status: [Beta]

<details>
<summary>Table of Contents</summary>

<!-- toc -->

- [Introduction](#introduction)
- [Communication Model](#communication-model)
  * [WebSocket Transport](#websocket-transport)
    + [WebSocket Message Format](#websocket-message-format)
    + [WebSocket Message Exchange](#websocket-message-exchange)
  * [Plain HTTP Transport](#plain-http-transport)
  * [AgentToServer and ServerToAgent Messages](#agenttoserver-and-servertoagent-messages)
    + [AgentToServer Message](#agenttoserver-message)
      - [AgentToServer.instance_uid](#agenttoserverinstance_uid)
      - [AgentToServer.sequence_num](#agenttoserversequence_num)
      - [AgentToServer.agent_description](#agenttoserveragent_description)
      - [AgentToServer.capabilities](#agenttoservercapabilities)
      - [AgentToServer.health](#agenttoserverhealth)
      - [AgentToServer.effective_config](#agenttoservereffective_config)
      - [AgentToServer.remote_config_status](#agenttoserverremote_config_status)
      - [AgentToServer.package_statuses](#agenttoserverpackage_statuses)
      - [AgentToServer.agent_disconnect](#agenttoserveragent_disconnect)
      - [AgentToServer.flags](#agenttoserverflags)
      - [AgentToServer.connection_settings_request](#agenttoserverconnection_settings_request)
      - [AgentToServer.custom_capabilities](#agenttoservercustom_capabilities)
      - [AgentToServer.custom_message](#agenttoservercustom_message)
      - [AgentToServer.available_components](#agenttoserveravailable_components)
      - [AgentToServer.connection_settings_status](#agenttoserverconnection_settings_status)
    + [ServerToAgent Message](#servertoagent-message)
      - [ServerToAgent.instance_uid](#servertoagentinstance_uid)
      - [ServerToAgent.error_response](#servertoagenterror_response)
      - [ServerToAgent.remote_config](#servertoagentremote_config)
      - [ServerToAgent.connection_settings](#servertoagentconnection_settings)
      - [ServerToAgent.packages_available](#servertoagentpackages_available)
      - [ServerToAgent.flags](#servertoagentflags)
      - [ServerToAgent.capabilities](#servertoagentcapabilities)
      - [ServerToAgent.agent_identification](#servertoagentagent_identification)
      - [ServerToAgent.command](#servertoagentcommand)
      - [ServerToAgent.custom_capabilities](#servertoagentcustom_capabilities)
      - [ServerToAgent.custom_message](#servertoagentcustom_message)
    + [ServerErrorResponse Message](#servererrorresponse-message)
      - [ServerErrorResponse.type](#servererrorresponsetype)
      - [ServerErrorResponse.error_message](#servererrorresponseerror_message)
      - [ServerErrorResponse.retry_info](#servererrorresponseretry_info)
  * [ServerToAgentCommand Message](#servertoagentcommand-message)
- [Operation](#operation)
  * [Status Reporting](#status-reporting)
    + [Agent Status Compression](#agent-status-compression)
    + [AgentDescription Message](#agentdescription-message)
      - [AgentDescription.identifying_attributes](#agentdescriptionidentifying_attributes)
      - [AgentDescription.non_identifying_attributes](#agentdescriptionnon_identifying_attributes)
    + [ComponentHealth Message](#componenthealth-message)
      - [ComponentHealth.healthy](#componenthealthhealthy)
      - [ComponentHealth.start_time_unix_nano](#componenthealthstart_time_unix_nano)
      - [ComponentHealth.last_error](#componenthealthlast_error)
      - [ComponentHealth.status](#componenthealthstatus)
      - [ComponentHealth.status_time_unix_nano](#componenthealthstatus_time_unix_nano)
      - [ComponentHealth.component_health_map](#componenthealthcomponent_health_map)
    + [EffectiveConfig Message](#effectiveconfig-message)
      - [EffectiveConfig.config_map](#effectiveconfigconfig_map)
    + [RemoteConfigStatus Message](#remoteconfigstatus-message)
      - [RemoteConfigStatus.last_remote_config_hash](#remoteconfigstatuslast_remote_config_hash)
      - [RemoteConfigStatus.status](#remoteconfigstatusstatus)
      - [RemoteConfigStatus.error_message](#remoteconfigstatuserror_message)
    + [ConnectionSettingsStatus Message](#connectionsettingsstatus-message)
      - [ConnectionSettingsStatus.last_connection_settings_hash](#connectionsettingsstatuslast_connection_settings_hash)
      - [ConnectionSettingsStatus.status](#connectionsettingsstatusstatus)
      - [ConnectionSettingsStatus.error_message](#connectionsettingsstatuserror_message)
    + [PackageStatuses Message](#packagestatuses-message)
      - [PackageStatuses.packages](#packagestatusespackages)
      - [PackageStatuses.server_provided_all_packages_hash](#packagestatusesserver_provided_all_packages_hash)
      - [PackageStatuses.error_message](#packagestatuseserror_message)
    + [PackageStatus Message](#packagestatus-message)
      - [PackageStatus.name](#packagestatusname)
      - [PackageStatus.agent_has_version](#packagestatusagent_has_version)
      - [PackageStatus.agent_has_hash](#packagestatusagent_has_hash)
      - [PackageStatus.server_offered_version](#packagestatusserver_offered_version)
      - [PackageStatus.server_offered_hash](#packagestatusserver_offered_hash)
      - [PackageStatus.status](#packagestatusstatus)
      - [PackageStatus.error_message](#packagestatuserror_message)
      - [PackageStatus.download_details](#packagestatusdownload_details)
  * [Connection Settings Management](#connection-settings-management)
    + [OpAMP Connection Setting Offer Flow](#opamp-connection-setting-offer-flow)
    + [Trust On First Use](#trust-on-first-use)
    + [Registration On First Use](#registration-on-first-use)
    + [Agent-initiated CSR Flow](#agent-initiated-csr-flow)
      - [Using instance_uid in the CSR](#using-instance_uid-in-the-csr)
    + [Revoking Access](#revoking-access)
    + [Certificate Generation](#certificate-generation)
    + [Connection Settings for "Other" Destinations](#connection-settings-for-other-destinations)
    + [ConnectionSettingsRequest Message](#connectionsettingsrequest-message)
    + [OpAMPConnectionSettingsRequest Message](#opampconnectionsettingsrequest-message)
    + [CertificateRequest Message](#certificaterequest-message)
    + [ConnectionSettingsOffers Message](#connectionsettingsoffers-message)
      - [ConnectionSettingsOffers.hash](#connectionsettingsoffershash)
      - [ConnectionSettingsOffers.opamp](#connectionsettingsoffersopamp)
      - [ConnectionSettingsOffers.own_metrics](#connectionsettingsoffersown_metrics)
      - [ConnectionSettingsOffers.own_traces](#connectionsettingsoffersown_traces)
      - [ConnectionSettingsOffers.own_logs](#connectionsettingsoffersown_logs)
      - [ConnectionSettingsOffers.other_connections](#connectionsettingsoffersother_connections)
    + [OpAMPConnectionSettings](#opampconnectionsettings)
      - [OpAMPConnectionSettings.destination_endpoint](#opampconnectionsettingsdestination_endpoint)
      - [OpAMPConnectionSettings.headers](#opampconnectionsettingsheaders)
      - [OpAMPConnectionSettings.certificate](#opampconnectionsettingscertificate)
      - [OpAMPConnectionSettings.heartbeat_interval_seconds](#opampconnectionsettingsheartbeat_interval_seconds)
      - [OpAMPConnectionSettings.tls](#opampconnectionsettingstls)
      - [OpAMPConnectionSettings.proxy](#opampconnectionsettingsproxy)
    + [TelemetryConnectionSettings](#telemetryconnectionsettings)
      - [TelemetryConnectionSettings.destination_endpoint](#telemetryconnectionsettingsdestination_endpoint)
      - [TelemetryConnectionSettings.headers](#telemetryconnectionsettingsheaders)
      - [TelemetryConnectionSettings.certificate](#telemetryconnectionsettingscertificate)
      - [TelemetryConnectionSettings.tls](#telemetryconnectionsettingstls)
      - [TelemetryConnectionSettings.proxy](#telemetryconnectionsettingsproxy)
    + [OtherConnectionSettings](#otherconnectionsettings)
      - [OtherConnectionSettings.destination_endpoint](#otherconnectionsettingsdestination_endpoint)
      - [OtherConnectionSettings.headers](#otherconnectionsettingsheaders)
      - [OtherConnectionSettings.certificate](#otherconnectionsettingscertificate)
      - [OtherConnectionSettings.other_settings](#otherconnectionsettingsother_settings)
      - [OtherConnectionSettings.tls](#otherconnectionsettingstls)
      - [OtherConnectionSettings.proxy](#otherconnectionsettingsproxy)
    + [TLSConnectionSettings Message](#tlsconnectionsettings-message)
      - [TLSConnectionSettings.ca_pem_contents](#tlsconnectionsettingsca_pem_contents)
      - [TLSConnectionSettings.include_system_ca_pool](#tlsconnectionsettingsinclude_system_ca_pool)
      - [TLSConnectionSettings.insecure_skip_verify](#tlsconnectionsettingsinsecure_skip_verify)
      - [TLSConnectionSettings.min_version](#tlsconnectionsettingsmin_version)
      - [TLSConnectionSettings.max_version](#tlsconnectionsettingsmax_version)
      - [TLSConnectionSettings.ciper_suites](#tlsconnectionsettingsciper_suites)
    + [ProxyConnectionSettings Message](#proxyconnectionsettings-message)
      - [ProxyConnectionSettings.url](#proxyconnectionsettingsurl)
      - [ProxyConnectionSettings.connect_headers](#proxyconnectionsettingsconnect_headers)
    + [Headers Message](#headers-message)
    + [TLSCertificate Message](#tlscertificate-message)
      - [TLSCertificate.cert](#tlscertificatecert)
      - [TLSCertificate.private_key](#tlscertificateprivate_key)
      - [TLSCertificate.ca_cert](#tlscertificateca_cert)
  * [Own Telemetry Reporting](#own-telemetry-reporting)
  * [Configuration](#configuration)
    + [Configuration Files](#configuration-files)
    + [Security Considerations](#security-considerations)
    + [AgentRemoteConfig Message](#agentremoteconfig-message)
  * [Packages](#packages)
    + [Downloading Packages](#downloading-packages)
      - [Step 1](#step-1)
      - [Step 2](#step-2)
      - [Step 3](#step-3)
    + [Package Status Reporting](#package-status-reporting)
    + [Calculating Hashes](#calculating-hashes)
      - [File Hash](#file-hash)
      - [Package Hash](#package-hash)
      - [All Packages Hash](#all-packages-hash)
    + [Security Considerations](#security-considerations-1)
    + [PackagesAvailable Message](#packagesavailable-message)
      - [PackagesAvailable.packages](#packagesavailablepackages)
      - [PackagesAvailable.all_packages_hash](#packagesavailableall_packages_hash)
    + [PackageAvailable Message](#packageavailable-message)
      - [PackageAvailable.type](#packageavailabletype)
      - [PackageAvailable.version](#packageavailableversion)
      - [PackageAvailable.file](#packageavailablefile)
      - [PackageAvailable.hash](#packageavailablehash)
    + [DownloadableFile Message](#downloadablefile-message)
      - [DownloadableFile.download_url](#downloadablefiledownload_url)
      - [DownloadableFile.content_hash](#downloadablefilecontent_hash)
      - [DownloadableFile.signature](#downloadablefilesignature)
      - [DownloadableFile.headers](#downloadablefileheaders)
  * [Custom Messages](#custom-messages)
    + [Motivation](#motivation)
    + [CustomCapabilities](#customcapabilities)
      - [CustomCapabilities.capabilities](#customcapabilitiescapabilities)
    + [CustomMessage](#custommessage)
      - [CustomMessage.capability](#custommessagecapability)
      - [CustomMessage.type](#custommessagetype)
      - [CustomMessage.data](#custommessagedata)
    + [Examples](#examples)
      - [Pause/Resume Example](#pauseresume-example)
        * [Agent Connection](#agent-connection)
        * [Pause](#pause)
        * [Resume](#resume)
      - [Service Discovery Example](#service-discovery-example)
        * [Agent Connection](#agent-connection-1)
        * [FindServices](#findservices)
        * [FindServicesResponse](#findservicesresponse)
  * [AvailableComponents Message](#availablecomponents-message)
    + [AvailableComponents.components](#availablecomponentscomponents)
      - [Examples](#examples-1)
        * [OpenTelemetry Collector](#opentelemetry-collector)
        * [Fluent Bit](#fluent-bit)
    + [AvailableComponents.hash](#availablecomponentshash)
    + [Initial Handshake](#initial-handshake)
    + [ComponentDetails Message](#componentdetails-message)
      - [ComponentDetails.metadata](#componentdetailsmetadata)
      - [ComponentDetails.sub_component_map](#componentdetailssub_component_map)
- [Connection Management](#connection-management)
  * [Establishing Connection](#establishing-connection)
  * [Closing Connection](#closing-connection)
    + [WebSocket Transport, OpAMP Client Initiated](#websocket-transport-opamp-client-initiated)
    + [WebSocket Transport, Server Initiated](#websocket-transport-server-initiated)
    + [Plain HTTP Transport](#plain-http-transport-1)
  * [Restoring WebSocket Connection](#restoring-websocket-connection)
  * [Duplicate WebSocket Connections](#duplicate-websocket-connections)
  * [Authentication](#authentication)
  * [Bad Request](#bad-request)
  * [Retrying Messages](#retrying-messages)
  * [Throttling](#throttling)
    + [WebSocket Transport](#websocket-transport-1)
    + [Plain HTTP Transport](#plain-http-transport-2)
- [Security](#security)
  * [General Recommendations](#general-recommendations)
  * [Configuration Restrictions](#configuration-restrictions)
  * [Opt-in Remote Configuration](#opt-in-remote-configuration)
  * [Code Signing](#code-signing)
- [Interoperability](#interoperability)
  * [Interoperability of Partial Implementations](#interoperability-of-partial-implementations)
  * [Interoperability of Future Capabilities](#interoperability-of-future-capabilities)
    + [Ignorable Capability Extensions](#ignorable-capability-extensions)
    + [Non-Ignorable Capability Extensions](#non-ignorable-capability-extensions)
    + [Protobuf Schema Stability](#protobuf-schema-stability)
- [Future Possibilities](#future-possibilities)
- [References](#references)
  * [Agent Management](#agent-management)
  * [Configuration Management](#configuration-management)
  * [Security and Certificate Management](#security-and-certificate-management)
  * [Cloud Provider Support](#cloud-provider-support)
  * [Other](#other)

<!-- tocstop -->

</details>

## Introduction

Open Agent Management Protocol (OpAMP) is a network protocol for remote
management of large fleets of data collection Agents.

OpAMP allows Agents to report their status to and receive configuration from a
Server and to receive Agent installation package updates from the
Server. The protocol is vendor-agnostic, so the Server can remotely monitor and
manage a fleet of different Agents that implement OpAMP, including a fleet of
mixed Agents from different vendors.

OpAMP supports the following functionality:

* Remote configuration of the Agents.
* Status reporting. The protocol allows the Agent to report the properties of
  the Agent such as its type and version or the operating system type and
  version it runs on. The status reporting also allows the management Server to
  tailor the remote configuration to individual Agents or types of Agents.
* Agent's own telemetry reporting to an
  [OTLP](https://opentelemetry.io/docs/specs/otlp/)-compatible
  backend to monitor Agent's process metrics such as CPU or RAM usage, as well
  as Agent-specific metrics such as rate of data processing.
* Agent heartbeating.
* Management of downloadable Agent-specific packages.
* Secure auto-updating capabilities (both upgrading and downgrading of the
  Agents).
* Connection credentials management, including client-side TLS certificate
  revocation and rotation.

The functionality listed above enables a 'single pane of glass' management view
of a large fleet of mixed Agents (e.g. OpenTelemetry Collector, Fluentd, etc).

## Communication Model

The OpAMP Server manages Agents that provide a client-side implementation of OpAMP
protocol, further referred to as OpAMP Client or simply Client.
OpAMP does not assume any particular relationship between the Agent and the Client.
The Client can be run as a separate process with a different lifecycle than the Agent,
a sidecar, a plugin, or be fully integrated into the Agent code.

The Agents can optionally send their own telemetry to an OTLP
destination when directed so by the OpAMP Server. The Agents likely also connect
to other destinations, where they send the data they collect:

```
        ┌────────────┬────────┐           ┌─────────┐
        │            │ OpAMP  │  OpAMP    │ OpAMP   │
        │            │        ├──────────►│         │
        │            │ Client │           │ Server  │
        │            └────────┤           └─────────┘
        │                     │
        │            ┌────────┤           ┌─────────┐
        │            │OTLP    │ OTLP/HTTP │ OTLP    │
        │  Agent     │        ├──────────►│         │
        │            │Exporter│           │ Receiver│
        │            └────────┤           └─────────┘
        │                     │
        │            ┌────────┤
        │            │Other   ├──────────► Other
        │            │Clients ├──────────► Destinations
        └────────────┴────────┘
```

This specification defines the OpAMP network protocol and the expected behavior
for OpAMP Agents and Servers. For the OTLP/HTTP protocol specification, see
[OTLP](https://opentelemetry.io/docs/specs/otlp/).
The protocols used by the Agent to connect to other destinations are Agent
type-specific and are outside the scope of this specification.

OpAMP protocol works over one of the 2 supported transports: plain HTTP
connections and WebSocket connections. Server implementations SHOULD accept both
plain HTTP connections and WebSocket connections. OpAMP Client implementations may
choose to support either plain HTTP or WebSocket transport, depending on their
needs.

OpAMP Clients connect to OpAMP Server on behalf of the Agents.
Typically a single Server accepts connections from many Clients. Agents
are identified by self-assigned or Server-assigned globally unique instance identifiers (or
instance_uid for short). The instance_uid is recorded in each message sent from
the Agent (via the Client) to the Server and from the Server to the Agent.

The default URL path for the connection is /v1/opamp. The URL path MAY be
configurable on the Client and on the Server.

One of the typical ways to implement OpAMP on the Agent side is by having a helping
Supervisor process, which controls the Agent process. The Supervisor is also
typically responsible for communicating with the OpAMP Server:

```
      ┌───────────────────┐
      │ Supervisor        │
      │                   │
      │          ┌────────┤           ┌────────┐
      │          │ OpAMP  │  OpAMP    │ OpAMP  │
      │          │        ├──────────►│        │
      │          │ Client │           │ Server │
      └────┬─────┴────────┘           └────────┘
           │
           │
           ▼
      ┌──────────┐
      │          │
      │  Agent   │
      │          │
      └──────────┘
```

The OpAMP specification does not require using a Supervisor and does not define how the
Supervisor and the Agent communicate.

In the rest of the OpAMP specification the term _Agent_ is used to refer to the entity
which implements the client portion of the OpAMP, regardless of whether or not a
Supervisor is part of that entity.

### WebSocket Transport

One of the supported transports for OpAMP protocol is
[WebSocket](https://datatracker.ietf.org/doc/html/rfc6455). The OpAMP Client
is a WebSocket client and the Server is a WebSocket Server. The Client and the Server
communicate using binary data WebSocket messages. The content of each WebSocket
message is an encoded `header`, followed by a
[binary encoded Protobuf](https://developers.google.com/protocol-buffers/docs/encoding)
message `data` (see [WebSocket Message Format](#websocket-message-format)).

On behalf of the Agent, the Client sends AgentToServer message data
and the Server sends ServerToAgent Protobuf message data:

```
        ┌────────────\ \────────┐                        ┌──────────────┐
        │            / /        │   Data:AgentToServer   │              │
        │            \ \ OpAmp  ├───────────────────────►│              │
        │     Agent  / /        │                        │    Server    │
        │            \ \ Client │   Data:ServerToAgent   │              │
        │            / /        │◄───────────────────────┤              │
        └────────────\ \────────┘                        └──────────────┘
```

#### WebSocket Message Format

The format of each WebSocket message is the following:

```
        ┌────────────┬────────────────────────────────────────┬───────────────────┐
        │ header     │ Varint encoded unsigned 64 bit integer │ 1-10 bytes        │
        ├────────────┼────────────────────────────────────────┼───────────────────┤
        │ data       │ Encoded Protobuf message,              │ 0 or more bytes   │
        │            │ either AgentToServer or ServerToAgent  │                   │
        └────────────┴────────────────────────────────────────┴───────────────────┘
```

The unencoded `header` is a 64 bit unsigned integer. In the WebSocket message the 64 bit
unencoded `header` value is encoded into bytes using [Base 128 Varint](https://developers.google.com/protocol-buffers/docs/encoding#varints) format. The
number of the bytes that the encoded `header` uses depends on the value of unencoded
`header` and can be anything between 1 and 10 bytes.

The value of the unencoded `header` is set equal to 0 in this version of the specification.
All other `header` values are reserved for future use. Such values will be defined in
future versions of OpAMP specification. OpAMP WebSocket message decoders that are
compliant with this specification SHOULD check that the value of the `header` is equal
to 0 and if it is not SHOULD assume that the WebSocket message is malformed.

The `data` field contains the bytes that represent the AgentToServer or ServerToAgent
message encoded in [Protobuf binary wire format](https://developers.google.com/protocol-buffers/docs/encoding).

Note that both `header` and `data` fields contain a variable number of bytes.
The decoding Base 128 Varint algorithm for the `header` knows when to stop based on the
bytes it reads.

To decode the `data` field using Protobuf decoding logic the implementation needs
to know the number of the bytes of the `data` field. To calculate this the implementation
MUST deduct the size of the `header` in bytes from the size of the WebSocket message
in bytes.

Note that due to the way Protobuf wire format is designed the size of the `data` in
bytes can be 0 if the encoded AgentToServer or ServerToAgent message is empty (i.e. all
fields are unset). This is a valid situation.

#### WebSocket Message Exchange

OpAMP over WebSocket is an asynchronous, full-duplex message exchange protocol. The order and
sequence of messages exchanged by the OpAMP Client and the Server is defined for each
particular capability in the corresponding section of this specification.

The sequence is normally started by an initiating message triggered by some
external event. For example after the connection is established the Client sends
an AgentToServer message. In this case the "connection established" is the
triggering event and the AgentToServer is the initiating message.

Both the Client and the Server may begin a sequence by sending an initiating
message.

The initiating message may trigger the recipient to send one or more messages
back, which in turn may trigger messages in the opposite direction and so on.
This exchange of messages in both directions continues until the sequence is
over because the goal of the exchange is achieved or the sequence failed with an
error.

Note that the same message may in some cases be the initiating message of the
sequence and in some other cases it may be triggered in response to receiving
some other message. Unlike other protocols in OpAMP there is no strict
separation between "request" and "response" messages types. The role of the
message depends on how the sequence is triggered.

For example the AgentToServer message may be the initiating message sent by the
Client when the Client connects to the Server for the first time. The AgentToServer
message may also be sent by the Client in response to the Server making a remote
configuration offer to the Agent and Agent reporting that it accepted the
configuration.

If the Client is capable of sending heartbeats the Client SHOULD set
ReportsHeartbeat capability. If ReportsHeartbeat capability is set the
Client SHOULD send heartbeats periodically. The interval between the
heartbeats SHOULD be 30 seconds, unless a different value is configured
on the Client side or unless a different interval is offered by the Server via
`OpAMPConnectionSettings.heartbeat_interval_seconds` field.

See sections under the [Operation](#operation) section for the details of the
message sequences.

The WebSocket transport is typically used when it is necessary to have instant
communication ability from the Server to the Agent without waiting for the Client
to poll the Server like it is done when using the HTTP transport (see below).

### Plain HTTP Transport

The second supported transport for OpAMP protocol is plain HTTP connection. The
OpAMP Client is an HTTP client and the Server is an HTTP server. The Client makes
POST requests to the Server. The body of the POST request and response is a
[binary serialized Protobuf](https://developers.google.com/protocol-buffers/docs/encoding)
message. The Client sends AgentToServer Protobuf messages in the request body and
the Server sends ServerToAgent Protobuf messages in the response body.

OpAMP over HTTP is a synchronous, half-duplex message exchange protocol. The
Client initiates an HTTP request when it has an AgentToServer message to deliver.
The Server responds to each HTTP request with a ServerToAgent message it wants
to deliver to the Agent. If the Agent has nothing to deliver to the Server the
Client MUST periodically poll the Server by sending an AgentToServer message
where only [instance_uid](#agenttoserverinstance_uid) field is set. This gives the Server an
opportunity to send back in the response any messages that the Server wants to
deliver to the Agent (such as for example a new remote configuration).

The default polling interval when the Agent does not have anything to deliver is 30
seconds. This polling interval SHOULD be configurable on the Client.
If the client has previously received and accepted OpAMP connection settings
then the value of `OpAMPConnectionSettings.heartbeat_interval_seconds`
SHOULD be used as the polling interval.

When using HTTP transport the sequence of messages is exactly the same as it is
when using the WebSocket transport. The only difference is in the timing:

- When the Server wants to send a message to the Agent, the Server needs to wait
  for the Client to poll the Server and establish an HTTP request over which the Server's
  message can be sent back as an HTTP response.
- When the Agent wants to send a message to the Server and the Agent has previously sent
  a request to the Server that is not yet responded, the Client MUST wait until the
  response is received before a new request can be made. Note that the new request in
  this case can be made immediately after the previous response is received, the Client
  does not need to wait for the polling period between requests.

The Client MUST set "Content-Type: application/x-protobuf" request header when
using plain HTTP transport. When the Server receives an HTTP request with this
header set it SHOULD assume this is a plain HTTP transport request, otherwise it
SHOULD assume this is a WebSocket transport initiation.

The Client MAY compress the request body using gzip method and MUST specify
"Content-Encoding: gzip" in that case. Server implementations MUST honour the
"Content-Encoding" header and MUST support gzipped or uncompressed request bodies.

The Server SHOULD compress the response if the Client indicated it can accept compressed
response via the "Accept-Encoding" header.

### AgentToServer and ServerToAgent Messages

#### AgentToServer Message

The body of the OpAMP WebSocket message or HTTP body of the request is a binary
serialized Protobuf message AgentToServer as defined below (all messages in this
document are specified in
[Protobuf 3 language](https://developers.google.com/protocol-buffers/docs/proto3)):

```protobuf
message AgentToServer {
    bytes instance_uid = 1;
    uint64 sequence_num = 2;
    AgentDescription agent_description = 3;
    uint64 capabilities = 4;
    ComponentHealth health = 5;
    EffectiveConfig effective_config = 6;
    RemoteConfigStatus remote_config_status = 7;
    PackageStatuses package_statuses = 8;
    AgentDisconnect agent_disconnect = 9;
    uint64 flags = 10;
    ConnectionSettingsRequest connection_settings_request = 11; // Status: [Development]
    CustomCapabilities custom_capabilities = 12; // Status: [Development]
    CustomMessage custom_message = 13; // Status: [Development]
    AvailableComponents available_components = 14; // Status: [Development]
    ConnectionSettingsStatus connection_settings_status = 15; // Status: [Development]
}
```

The Server should process each field as it is described in the corresponding
[Operation](#operation) section.

##### AgentToServer.instance_uid

The instance_uid field is a globally unique identifier of the running instance
of the Agent. The Agent SHOULD self-generate this identifier and make the best
effort to avoid creating an identifier that may conflict with identifiers
created by other Agents. The instance_uid SHOULD remain unchanged for the
lifetime of the Agent process. The instance_uid MUST be
16 bytes long and SHOULD be generated using the
[UUID v7 spec](https://www.ietf.org/archive/id/draft-ietf-uuidrev-rfc4122bis-14.html#name-uuid-version-7).

In case the Agent wants to use an identifier generated by the Server, the field
SHOULD be set with a temporary value and RequestInstanceUid flag MUST be set.

##### AgentToServer.sequence_num

The sequence number is incremented by 1 for every AgentToServer message sent
by the Client. This allows the Server to detect that it missed a message when
it notices that the sequence_num is not exactly by 1 greater than the previously
received one. See [Agent Status Compression](#agent-status-compression) for more
details.

##### AgentToServer.agent_description

Data that describes the Agent, its type, where it runs, etc. See
[AgentDescription](#agentdescription-message) message for details.
This field SHOULD be unset if this information is unchanged since the last
AgentToServer message.

##### AgentToServer.capabilities

Bitmask of flags defined by AgentCapabilities enum.
All bits that are not defined in AgentCapabilities enum MUST be set to 0 by
the Client. This allows extending the protocol and the AgentCapabilities enum
in the future such that old Agents automatically report that they don't
support the new capability.
This field MUST be always set.

An Agent MAY update any of its capabilities at any time after the first
message. The server MUST respect the Agent's changes, ensuring that the
subsequent messages are sent correctly. If the server sends a part of its
messages corresponding to a capability the Agent doesn't support, the Agent
SHOULD ignore it.

```protobuf
enum AgentCapabilities {
    // The capabilities field is unspecified.
    UnspecifiedAgentCapability = 0;
    // The Agent can report status. This bit MUST be set, since all Agents MUST
    // report status.
    ReportsStatus                   = 0x00000001;
    // The Agent can accept remote configuration from the Server.
    AcceptsRemoteConfig             = 0x00000002;
    // The Agent will report EffectiveConfig in AgentToServer.
    ReportsEffectiveConfig          = 0x00000004;
    // The Agent can accept package offers.
    // Status: [Beta]
    AcceptsPackages                 = 0x00000008;
    // The Agent can report package status.
    // Status: [Beta]
    ReportsPackageStatuses          = 0x00000010;
    // The Agent can report own trace to the destination specified by
    // the Server via ConnectionSettingsOffers.own_traces field.
    // Status: [Beta]
    ReportsOwnTraces                = 0x00000020;
    // The Agent can report own metrics to the destination specified by
    // the Server via ConnectionSettingsOffers.own_metrics field.
    // Status: [Beta]
    ReportsOwnMetrics               = 0x00000040;
    // The Agent can report own logs to the destination specified by
    // the Server via ConnectionSettingsOffers.own_logs field.
    // Status: [Beta]
    ReportsOwnLogs                  = 0x00000080;
    // The can accept connections settings for OpAMP via
    // ConnectionSettingsOffers.opamp field.
    // Status: [Beta]
    AcceptsOpAMPConnectionSettings  = 0x00000100;
    // The can accept connections settings for other destinations via
    // ConnectionSettingsOffers.other_connections field.
    // Status: [Beta]
    AcceptsOtherConnectionSettings  = 0x00000200;
    // The Agent can accept restart requests.
    // Status: [Beta]
    AcceptsRestartCommand           = 0x00000400;
    // The Agent will report Health via AgentToServer.health field.
    ReportsHealth                   = 0x00000800;
    // The Agent will report RemoteConfig status via AgentToServer.remote_config_status field.
    ReportsRemoteConfig             = 0x00001000;
    // The Agent can report heartbeats.
    // This is specified by the ServerToAgent.OpAMPConnectionSettings.heartbeat_interval_seconds field.
    // If this capability is true, but the Server does not set a heartbeat_interval_seconds field, the
    // Agent should use its own configured interval, which by default will be 30s. The Server may not
    // know the configured interval and should not make assumptions about it.
    // Status: [Development]
    ReportsHeartbeat                = 0x00002000;
    // The agent will report AvailableComponents via the AgentToServer.available_components field.
    // Status: [Development]
    ReportsAvailableComponents      = 0x00004000;
    // The agent will report ConnectionSettingsOffers status via AgentToServer.connection_settings_status field.
    // Status: [Development]
    ReportsConnectionSettingsStatus = 0x00008000;

    // Add new capabilities here, continuing with the least significant unused bit.
}
```

##### AgentToServer.health

Status: [Beta]

The current health of the Agent and sub-components. The top-level ComponentHealth
represents the health of the Agent overall. May be omitted if nothing changed since last
AgentToServer message. See [ComponentHealth message](#componenthealth-message).

##### AgentToServer.effective_config

The current effective configuration of the Agent. The effective configuration is
the one that is currently used by the Agent. The effective configuration may be
different from the remote configuration received from the Server earlier, e.g.
because the Agent uses a local configuration instead (or in addition). See
[EffectiveConfig](#effectiveconfig-message) message for details.
This field SHOULD be unset if this information is unchanged since the last
AgentToServer message.

##### AgentToServer.remote_config_status

The status of the remote config that was previously received from the Server.
See [RemoteConfigStatus](#remoteconfigstatus-message) message for details.
This field SHOULD be unset if this information is unchanged since the last
AgentToServer message.

##### AgentToServer.package_statuses

Status: [Beta]

The list of the Agent packages, including package statuses.
This field SHOULD be unset if this information is unchanged since the last
AgentToServer message.

##### AgentToServer.agent_disconnect

AgentDisconnect MUST be set in the last AgentToServer message sent from the
Client to the Server.

##### AgentToServer.flags

Bit flags as defined by AgentToServerFlags bit masks.

```protobuf
enum AgentToServerFlags {
    FlagsUnspecified = 0;

    // Flags is a bit mask. Values below define individual bits.

    // The Agent requests Server go generate a new instance_uid, which will
    // be sent back in ServerToAgent message
    RequestInstanceUid     = 0x00000001;
}
```

##### AgentToServer.connection_settings_request

Status: [Development]

A request to create connection settings. This field is set for flows where
the Agent initiates the creation of connection settings.

See [ConnectionSettingsRequest](#connectionsettingsrequest-message) message for details.

##### AgentToServer.custom_capabilities

Status: [Development]

A message indicating custom capabilities supported by the Agent.

See [CustomCapabilities](#customcapabilities) message for details.

##### AgentToServer.custom_message

Status: [Development]

A custom message sent from an Agent to the Server.

See [CustomMessage](#custommessage) message for details.

##### AgentToServer.available_components

Status: [Development]

A message listing the components available in the Agent. This field SHOULD be
reported if and only if the ReportsAvailableComponents capability is set.

See [AvailableComponents](#availablecomponents-message) message for details.

##### AgentToServer.connection_settings_status

Status: [Development]

The status of the connection settings that was previously recieved from the
Server. See [ConnectionSettingsStatus](#connectionsettingsstatus-message)
message for details. This field SHOULD be unset if this information is unchanged
since the last AgentToServer message. This field is a part of the
[Connection Settings Management](#connection-settings-management) workflow.

#### ServerToAgent Message

The body of the WebSocket message or HTTP response body is a binary serialized
Protobuf message ServerToAgent.

ServerToAgent message is sent from the Server to the Agent either in response to
the AgentToServer message or when the Server has data to deliver to the Agent.

If the Server receives an AgentToServer message and the Server has no data to
send back to the Agent then ServerToAgent message will still be sent, but all
fields except instance_uid will be unset (in that case ServerToAgent serves
simply as an acknowledgement of receipt).

Upon receiving a ServerToAgent message the Agent MUST process it. The processing
that needs to be performed depends on what fields in the message are set. For
details see links to the corresponding sections of this specification from the
field descriptions below.

As a result of this processing the Agent may need to send status reports to the
Server. The Agent is free to perform all the processing of the ServerToAgent
message completely and then send one status report or it may send multiple
status reports as it processes the portions of ServerToAgent message to indicate
the progress (see e.g. [Downloading Packages](#downloading-packages)). Multiple status
reports may be desirable when processing takes a long time, in which case the
status reports allow the Server to stay informed.

Note that the Server will reply to each status report with a ServerToAgent
message (or with an ServerErrorResponse if something goes wrong). These
ServerToAgent messages may have the same content as the one received earlier or
the content may be different if the situation on the Server has changed. The
Agent SHOULD be ready to process these additional ServerToAgent messages as they
arrive.

The Client SHOULD NOT send any status reports at all if the status of the Agent
did not change as a result of processing.

The ServerToAgent message has the following structure:

```protobuf
message ServerToAgent {
    bytes instance_uid = 1;
    ServerErrorResponse error_response = 2;
    AgentRemoteConfig remote_config = 3;
    ConnectionSettingsOffers connection_settings = 4; // Status: [Beta]
    PackagesAvailable packages_available = 5; // Status: [Beta]
    uint64 flags = 6;
    uint64 capabilities = 7;
    AgentIdentification agent_identification = 8;
    ServerToAgentCommand command = 9; // Status: [Beta]
    CustomCapabilities custom_capabilities = 10; // Status: [Development]
    CustomMessage custom_message = 11; // Status: [Development]
}
```

##### ServerToAgent.instance_uid

The Agent instance identifier. MUST match the instance_uid field previously
received in the AgentToServer message. When communication with multiple Agents
is multiplexed into one WebSocket connection (for example when a terminating
proxy is used) the instance_uid field allows to distinguish which Agent the
ServerToAgent message is addressed to.

Note: the value can be overridden by Server by sending a new one in the
AgentIdentification field. When this happens then Agent MUST update its
instance_uid to the value provided and use it for all further communication.

##### ServerToAgent.error_response

error_response is set if the Server wants to indicate that something went wrong
during processing of an AgentToServer message. If error_response is set then all
other fields below must be unset and vice versa, if any of the fields below is
set then error_response must be unset.

##### ServerToAgent.remote_config

This field is set when the Server has a remote config offer for the Agent. See
[Configuration](#configuration) for details.

##### ServerToAgent.connection_settings

Status: [Beta]

This field is set when the Server wants the Agent to change one or more of its
client connection settings (destination, headers, certificate, etc). See
[Connection Settings Management](#connection-settings-management) for details.

##### ServerToAgent.packages_available

Status: [Beta]

This field is set when the Server has packages to offer to the Agent. See
[Packages](#packages) for details.

##### ServerToAgent.flags

Bit flags as defined by ServerToAgentFlags bit masks.

`Report*` flags can be used by the Server if the Client did not include the
particular portion of the data in the last AgentToServer message (which is an allowed
compression approach) but the Server does not have that data, e.g. the Server was
restarted and lost the agent status (see the details in
[this section](#agent-status-compression)).

```protobuf
enum Flags {
    FlagsUnspecified = 0;

    // Flags is a bit mask. Values below define individual bits.

    // ReportFullState flag can be used by the Server if the Client did not include
    // some sub-message in the last AgentToServer message (which is an allowed
    // optimization) but the Server detects that it does not have it (e.g. was
    // restarted and lost state). The detection happens using
    // AgentToServer.sequence_num values.
    // The Server asks the Agent to report the full status again by sending
    // a new, full AgentToServer message.
    ReportFullState = 0x00000001;
    // ReportAvailableComponents flag can be used by the server if the Agent did
    // not include the full AvailableComponents message, but only the hash.
    // If this flag is specified, the agent will populate available_components.components
    // with a full description of the agent's components.
    // Status: [Development]
    ServerToAgentFlags_ReportAvailableComponents = 0x00000002;
}
```

##### ServerToAgent.capabilities

Bitmask of flags defined by ServerCapabilities enum. All bits that are not
defined in ServerCapabilities enum MUST be set to 0 by the Server. This allows
extending the protocol and the ServerCapabilities enum in the future such that
old Servers automatically report that they don't support the new capability.
This field MUST be set in the first ServerToAgent sent by the Server and MAY be
omitted in subsequent ServerToAgent messages by setting it to
UnspecifiedServerCapability value.

```protobuf
enum ServerCapabilities {
    // The capabilities field is unspecified.
    UnspecifiedServerCapability = 0;
    // The Server can accept status reports. This bit MUST be set, since all Server
    // MUST be able to accept status reports.
    AcceptsStatus                  = 0x00000001;
    // The Server can offer remote configuration to the Agent.
    OffersRemoteConfig             = 0x00000002;
    // The Server can accept EffectiveConfig in AgentToServer.
    AcceptsEffectiveConfig         = 0x00000004;
    // The Server can offer Packages.
    OffersPackages                 = 0x00000008;
    // The Server can accept Packages status.
    // Status: [Beta]
    AcceptsPackagesStatus          = 0x00000010;
    // The Server can offer connection settings.
    // Status: [Beta]
    OffersConnectionSettings       = 0x00000020;
    // The Server can accept ConnectionSettingsRequest and respond with an offer.
    // Status: [Development]
    AcceptsConnectionSettingsRequest = 0x00000040;

    // Add new capabilities here, continuing with the least significant unused bit.
}
```

##### ServerToAgent.agent_identification

Properties related to identification of the Agent, which can be overridden by the
Server if needed. When new_instance_uid is set, Agent MUST update instance_uid
to the value provided and use it for all further communication. The new_instance_uid MUST
be 16 bytes long and SHOULD be generated using the
[UUID v7 spec](https://www.ietf.org/archive/id/draft-ietf-uuidrev-rfc4122bis-14.html#name-uuid-version-7).

```protobuf
message AgentIdentification {
  bytes new_instance_uid = 1;
}
```

##### ServerToAgent.command

Status: [Beta]

This field is set when the Server wants the Agent to
perform a restart. This field must not be set with other fields
besides instance_uid or capabilities. All other fields will be ignored and the
Agent will execute the command. See [ServerToAgentCommand Message](#servertoagentcommand-message)
for details.

##### ServerToAgent.custom_capabilities

Status: [Development]

A message indicating custom capabilities supported by the Server.

See [CustomCapabilities](#customcapabilities) message for details.

##### ServerToAgent.custom_message

Status: [Development]

A custom message sent from the Server to an Agent.

See [CustomMessage](#custommessage) message for details.

#### ServerErrorResponse Message

The message has the following structure:

```protobuf
message ServerErrorResponse {
    enum Type {
        UNKNOWN = 0;
        BAD_REQUEST = 1;
        UNAVAILABLE = 2
    }
    Type type = 1;
    string error_message = 2;
    oneof Details {
        RetryInfo retry_info = 3;
    }
}
```

##### ServerErrorResponse.type

This field defines the type of the error that the Server encountered when trying
to process the Agent's request. Possible values are:

UNKNOWN: Unknown error. Something went wrong, but it is not known what exactly.
The error_message field may contain a description of the problem.

BAD_REQUEST: Only sent as a response to a previously received AgentToServer
message and indicates that the AgentToServer message was malformed. See
[Bad Request](#bad-request) processing.

UNAVAILABLE: The Server is overloaded and unable to process the request. See
[Throttling](#throttling).

##### ServerErrorResponse.error_message

Error message, typically human readable.

##### ServerErrorResponse.retry_info

Additional [RetryInfo](#throttling) message about retrying if type==UNAVAILABLE.

### ServerToAgentCommand Message

Status: [Beta]

The message has the following structure:

```protobuf
// ServerToAgentCommand is sent from the Server to the Agent to request that the Agent
// perform a command.
message ServerToAgentCommand {
    enum CommandType {
        // The Agent should restart. This request will be ignored if the Agent does not
        // support restart.
        Restart = 0;
    }
    CommandType type = 1;
}
```

The ServerToAgentCommand message is sent when the Server wants the Agent to restart.
This message must only contain the command, instance_uid, and capabilities fields. All other fields
will be ignored.

## Operation

### Status Reporting

The Client MUST send a status report:

* First time immediately after connecting to the Server. The status report MUST
  be the first message sent by the Client.
* Subsequently, every time the status of the Agent changes.

The status report is sent as an [AgentToServer](#agenttoserver-message) message.
The following fields in the message can be set to reflect the corresponding
part of the status: agent_description, capabilities, health, effective_config,
remote_config_status, package_statuses.

The Server MUST respond to the AgentToServer message by sending a
[ServerToAgent](#servertoagent-message) message.

If the status report processing failed then the
[error_response](#servertoagenterror_response) field MUST be set to ServerErrorResponse
message.

If the status report is processed successfully by the Server then the
[error_response](#servertoagenterror_response) field MUST be unset and the other fields
can be populated as necessary.

Here is the sequence diagram that shows how status reporting works (assuming
server-side processing is successful):

```
        Client                                  Server

          │                                       │
          │                                       │
          │          WebSocket Connect            │
          ├──────────────────────────────────────►│
          │                                       │
          │           AgentToServer               │   ┌─────────┐
          ├──────────────────────────────────────►├──►│         │
          │                                       │   │ Process │
          │           ServerToAgent               │   │ Status  │
          │◄──────────────────────────────────────┤◄──┤         │
          │                                       │   └─────────┘
          .                 ...                   .

          │           AgentToServer               │   ┌─────────┐
          ├──────────────────────────────────────►├──►│         │
          │                                       │   │ Process │
          │           ServerToAgent               │   │ Status  │
          │◄──────────────────────────────────────┤◄──┤         │
          │                                       │   └─────────┘
          │                                       │
```

Note that the status of the Agent may change as a result of receiving a message
from the Server. For example the Server may send a remote configuration to the
Agent. Once the Agent processes such a request the Agent's status changes (e.g.
the effective configuration of the Agent changes). Such status change should
result in the Client sending a status report to the Server.

So, essentially in such cases the sequence of messages may look like this:

```
          Agent   Client                                  Server

            │       │         ServerToAgent                 │
            ┌───────┤◄──────────────────────────────────────┤
            │       │                                       │
            ▼       │                                       │
        ┌────────┐  │                                       │
        │Process │  │                                       │
        │Received│  │                                       │
        │Data    │  │                                       │
        └───┬────┘  │                                       │
            │       │                                       │
            │Status │                                       │
            │Changed│         AgentToServer                 │   ┌─────────┐
            └──────►├──────────────────────────────────────►├──►│         │
                    │                                       │   │ Process │
                    │         ServerToAgent                 │   │ Status  │
                    │◄──────────────────────────────────────┤◄──┤         │
                    │                                       │   └─────────┘
```

When the Client receives a ServerToAgent message the Client MUST NOT send a status
report unless processing of the message received from the Server resulted in
actual change of the Agent status (e.g. the configuration of the Agent has
changed). The sequence diagram in this case look like this:

```
              Agent  Client                                  Server

                │      │         ServerToAgent                 │
                ┌──────┤◄──────────────────────────────────────┤
                │      │                                       │
                ▼      │                                       │
            ┌────────┐ │                                       │
            │Process │ │                                       │
            │Received│ │                                       │
            │Data    │ │                                       │
            └───┬────┘ │                                       │
                │      │                                       │
                ▼      │                                       │
             No Status │                                       │
              Changes  │                                       │
                       │                                       │
                       │                                       │
```

Important: if the Client does not follow these rules the operation may result in
an infinite loop of messages sent back and forth between the Client and the
Server.

#### Agent Status Compression

The Client notifies the Server about Agent's status by sending AgentToServer messages.
The status for example includes the Agent description, its effective configuration,
the status of the remote configuration it received from the Server and the status
of the packages. The Server tracks the status of the Agent using the data
specified in the sub-messages referenced from AgentToServer message.

The Client MAY compress the AgentToServer message by omitting the sub-messages that have not changed
since that particular data was reported last time. The following sub-messages can be subject
to such compression:
[AgentDescription](#agentdescription-message),
[ComponentHealth](#componenthealth-message),
[EffectiveConfig](#effectiveconfig-message),
[RemoteConfigStatus](#remoteconfigstatus-message),
[PackageStatuses](#packagestatuses-message),
[CustomCapabilities](#customcapabilities),
[AvailableComponents](#availablecomponents-message), and
[ConnectionSettingsStatus](#connectionsettingsstatus-message).

The compression is done by omitting the sub-message in the AgentToServer message.
If any of the fields in the sub-message has changed then the compression cannot be used
for that particular sub-message and the sub-message with all its relevant fields MUST
be present.

If all AgentToServer messages are reliably delivered to the Server and the Server
correctly processes them then such compression is safe and the Server should always
have the correct latest status of the Agent.

However, it is possible that the Client and Server lose the synchronization and the Client
believes the Server has the latest data while in reality the Server doesn't. This is
possible for example if the Server is restarted while the Client keeps running and sends
AgentToServer messages, which the Server does not receive because it is temporarily down.

In order to detect this situation and recover from it, the AgentToServer message
contains a sequence_num field. The field is an integer number that is incremented
every time the Client has a new AgentToServer message to send.

When the Server receives an AgentToServer message sequence_num field value that is not
exactly by one greater than the previously received sequence_num value then the Server
knows it does not have full status of the AgentToServer message.

When this situation is encountered, to recover the lost status the Server MUST request
the Agent to report the omitted data. To make this request the Server MUST send
a ServerToAgent message to the Agent and set the `ReportFullState` bit in
the [flags](#servertoagentflags) field of [ServerToAgent message](#servertoagent-message).

#### AgentDescription Message

The AgentDescription message has the following structure:

```protobuf
message AgentDescription {
    repeated KeyValue identifying_attributes = 1;
    repeated KeyValue non_identifying_attributes = 2;
}
```

##### AgentDescription.identifying_attributes

Attributes that identify the Agent.

Keys/values are according to OpenTelemetry [resource semantic
conventions](https://opentelemetry.io/docs/specs/semconv/resource/).

For standalone running Agents (such as OpenTelemetry Collector) the following
attributes SHOULD be specified:

- service.name should be set to the same value that the Agent uses in its own telemetry.
- service.namespace if it is used in the environment where the Agent runs.
- service.version should be set to version number of the Agent build.
- service.instance.id should be set. It may be set equal to the Agent's
  instance uid (equal to ServerToAgent.instance_uid field) or any other value
  that uniquely identifies the Agent in combination with other attributes.
- any other attributes that are necessary for uniquely identifying the Agent's
  own telemetry.

The Agent SHOULD also include these attributes in the Resource of its own
telemetry. The combination of identifying attributes SHOULD be sufficient to
uniquely identify the Agent's own telemetry in the destination system to which
the Agent sends its own telemetry.

##### AgentDescription.non_identifying_attributes

Attributes that do not necessarily identify the Agent but help describe where it
runs.

The following attributes SHOULD be included:

- os.type, os.version - to describe where the Agent runs.
- host.\* to describe the host the Agent runs on.
- cloud.\* to describe the cloud where the host is located.
- any other relevant Resource attributes that describe this Agent and the
  environment it runs in.
- any user-defined attributes that the end user would like to associate with
  this Agent.

#### ComponentHealth Message

Status: [Beta]

The ComponentHealth message has the following structure:

```protobuf
message ComponentHealth {
    bool healthy = 1;
    fixed64 start_time_unix_nano = 2;
    string last_error = 3;
    string status = 4;
    fixed64 status_time_unix_nano = 5;
    map<string, ComponentHealth> component_health_map = 6;
}
```

##### ComponentHealth.healthy

Set to true if the Agent is up and healthy.

##### ComponentHealth.start_time_unix_nano

Timestamp since the Agent is up, i.e. when the agent was started.
Value is UNIX Epoch time in nanoseconds since 00:00:00 UTC on 1 January 1970.
If the agent is not running MUST be set to 0.

##### ComponentHealth.last_error

Human-readable error message if the Agent is in erroneous state. SHOULD be set
when healthy==false.

##### ComponentHealth.status

Component status represented as a string. The status values are defined by agent-specific
semantics and not at the protocol level.

##### ComponentHealth.status_time_unix_nano

The time when the component status was observed. Value is UNIX Epoch time in
nanoseconds since 00:00:00 UTC on 1 January 1970.

##### ComponentHealth.component_health_map

A map to store more granular, sub-component health. It can nest as deeply as needed to
describe the underlying system.

#### EffectiveConfig Message

The EffectiveConfig message has the following structure:

```protobuf
message EffectiveConfig {
    AgentConfigMap config_map = 1;
}
```

##### EffectiveConfig.config_map

The effective config of the Agent.

See AgentConfigMap message definition in the [Configuration](#configuration)
section.

#### RemoteConfigStatus Message

The RemoteConfigStatus message has the following structure:

```protobuf
message RemoteConfigStatus {
    bytes last_remote_config_hash = 1;
    enum Status {
        // The value of status field is not set.
        UNSET = 0;

        // Remote config was successfully applied by the Agent.
        APPLIED = 1;

        // Agent is currently applying the remote config that it received earlier.
        APPLYING = 2;

        // Agent tried to apply the config received earlier, but it failed.
        // See error_message for more details.
        FAILED = 3;
    }
    Status status = 2;
    string error_message = 3;
}
```

##### RemoteConfigStatus.last_remote_config_hash

The hash of the remote config that was last received by this Agent in the
AgentRemoteConfig.config_hash field. The Server SHOULD compare this hash with the
config hash it has for the Agent and if the hashes are different the Server MUST
include the remote_config field in the response in the ServerToAgent message.

##### RemoteConfigStatus.status

The status of the Agent's attempt to apply a previously received remote
configuration.

##### RemoteConfigStatus.error_message

Optional error message if status==FAILED.

#### ConnectionSettingsStatus Message

The ConnectionSettingsStatus message has the following structure:

```protobuf
message ConnectionSettingsStatus {
    bytes last_connection_settings_hash = 1;
    enum Status {
        // The value of status field is not set.
        UNSET = 0;

        // offered connection settings were successfully applied by the Agent.
        APPLIED = 1;

        // Agent is currently applying the offered connection settings that it received earlier.
        APPLYING = 2;

        // Agent tried to apply the offered connection settings recieved earlier, but it failed.
        // See error_message for more details.
        FAILED = 3;
    }
    Status status = 2;
    string error_message = 3;
}
```

##### ConnectionSettingsStatus.last_connection_settings_hash

The hash of the offered connection settings that was last received by this Agent
in the connection_settings.hash field. The Server SHOULD compare this hash with
the config hash it has for the Agent and if the hashes are different the Server
MUST include the connection_settings field in the response in the ServerToAgent
message.

##### ConnectionSettingsStatus.status

The status of the Agent's attempt to apply previously received connection
settings.

##### ConnectionSettingsStatus.error_message

Optional error message if status==FAILED.

#### PackageStatuses Message

Status: [Beta]

The PackageStatuses message describes the status of all packages that the Agent
has or was offered. The message has the following structure:

```protobuf
message PackageStatuses {
    map<string, PackageStatus> packages = 1;
    bytes server_provided_all_packages_hash = 2;
    string error_message = 3;
}
```

##### PackageStatuses.packages

A map of PackageStatus messages, where the keys are package names. The key MUST
match the name field of [PackageStatus](#packagestatus-message) message.

##### PackageStatuses.server_provided_all_packages_hash

The aggregate hash of all packages that this Agent previously received from the
Server via PackagesAvailable message.

The Server SHOULD compare this hash to the aggregate hash of all packages that it
has for this Agent and if the hashes are different the Server SHOULD send an
PackagesAvailable message to the Agent.

##### PackageStatuses.error_message

This field is set if the Agent encountered an error when processing the
[PackagesAvailable message](#packagesavailable-message)
and that error is not related to any particular single package.

The field must be unset is there were no processing errors.

#### PackageStatus Message

Status: [Beta]

The PackageStatus has the following structure:

```protobuf
message PackageStatus {
    string name = 1;
    string agent_has_version = 2;
    bytes agent_has_hash = 3;
    string server_offered_version = 4;
    bytes server_offered_hash = 5;
    enum Status {
        INSTALLED = 0;
        INSTALL_PENDING = 1;
        INSTALLING = 2;
        INSTALL_FAILED = 3;
        DOWNLOADING = 4;
    }
    Status status = 6;
    string error_message = 7;
    PackageDownloadDetails download_details = 8;
}
```

##### PackageStatus.name

Package name. MUST be always set and MUST match the key in the packages field of
PackageStatuses message.

##### PackageStatus.agent_has_version

The version of the package that the Agent has.

MUST be set if the Agent has this package.

MUST be empty if the Agent does not have this package. This may be the case for
example if the package was offered by the Server but failed to install and the
Agent did not have this package previously.

##### PackageStatus.agent_has_hash

The hash of the package that the Agent has.

MUST be set if the Agent has this package.

MUST be empty if the Agent does not have this package. This may be the case for
example if the package was offered by the Server but failed to install and the
Agent did not have this package previously.

##### PackageStatus.server_offered_version

The version of the package that the Server offered to the Agent.

MUST be set if the installation of the package is initiated by an earlier offer
from the Server to install this package.

MUST be empty if the Agent has this package but it was installed locally and was
not offered by the Server.

Note that it is possible for both agent_has_version and server_offered_version
fields to be set and to have different values. This is for example possible if
the Agent already has a version of the package successfully installed, the Server
offers a different version, but the Agent fails to install that version.

##### PackageStatus.server_offered_hash

The hash of the package that the Server offered to the Agent.

MUST be set if the installation of the package is initiated by an earlier offer
from the Server to install this package.

MUST be empty if the Agent has this package but it was installed locally and was
not offered by the Server.

Note that it is possible for both agent_has_hash and server_offered_hash fields
to be set and to have different values. This is for example possible if the
Agent already has a version of the package successfully installed, the Server
offers a different version, but the Agent fails to install that version.

##### PackageStatus.status

The status of this package. The possible values are:

INSTALLED: Package is successfully installed by the Agent. The error_message field
MUST NOT be set.

INSTALLING: Agent is currently downloading and installing the package.
server_offered_hash field MUST be set to indicate the version that the Agent is
installing. The error_message field MUST NOT be set.

INSTALL_FAILED: Agent tried to install the package but installation failed.
server_offered_hash field MUST be set to indicate the version that the Agent
tried to install. The error_message may also contain more details about the
failure.

##### PackageStatus.error_message

An error message if the status is erroneous.

##### PackageStatus.download_details

Status: [Development]

The download_details contains additional details that descibe a package download.
It should only be set if the status is `DOWNLOADING`.

```protobuf
message PackageDownloadDetails {
  double download_percent = 1;
  double download_bytes_per_second = 2;
}
```

### Connection Settings Management

Status: [Beta]

OpAMP includes features that allow the Server to manage Agent's connection
settings for all of the destinations that the Agent connects to,
as well as the OpAMP Client's connection settings.

The following diagram shows a typical Agent that is managed by OpAMP Servers,
sends its own telemetry to an OTLP backend and also connects to other
destinations to perform its work:

```
            ┌────────────┬────────┐           ┌─────────┐
            │            │ OpAMP  │  OpAMP    │ OpAMP   │
            │            │        ├──────────►│         │
            │            │ Client │           │ Server  │
            │            └────────┤           └─────────┘
            │                     │
            │            ┌────────┤           ┌─────────┐
            │            │OTLP    │ OTLP/HTTP │OTLP     │
            │  Agent     │        ├──────────►│Telemetry│
            │            │Client  │           │Backend  │
            │            └────────┤           └─────────┘
            │                     │
            │            ┌────────┤
            │            │Other   ├──────────► Other
            │            │        ├──────────►
            │            │Clients ├──────────► Destinations
            └────────────┴────────┘
```

When connecting to the OpAMP Server and to other destinations it is typically
expected that Agents (or OpAMP Clients connecting on Agent's behalf) will use
some sort of header-based authorization mechanism
(e.g. an "Authorization" HTTP header or an access token in a custom header) and
optionally also client-side certificates for TLS connections (also known as
mutual TLS).

OpAMP protocol allows the Server to offer settings for each of these connections
and for the Agent to accept or reject such offers. This mechanism can be used to
direct the Agent to a specific destination, as well as for access token and TLS
certificate registration, revocation and rotation as needed.

The Server can offer connection settings for the following 3 classes of
destinations:

1. The **OpAMP Server** itself. This is typically used to manage credentials
   such as the TLS certificate or the request headers that are used for
   authorization. The Server MAY also offer a different destination endpoint to
   direct the OpAMP Client to connect to a different OpAMP Server.
2. The destinations for the Agent to send its **own telemetry**: metrics, traces
   and logs using OTLP/HTTP protocol.
3. A set of **additional "other" connection** settings, with a string name
   associated with each. How the Agent type uses these is Agent-specific.
   Typically the name represents the name of the destination to connect to (as
   it is known to the Agent). For example OpenTelemetry Collector can use the
   named connection settings for its exporters, one named connection setting per
   correspondingly named exporter.

The Server may make an offer for the particular connection class only if the
corresponding capability to use the connection is reported by the Agent via
AgentToServer.capabilities field:

- If ReportsOwnTraces capability bit is set the Server may offer connection
  settings for traces using own_traces field.
- If ReportsOwnMetrics capability bit is set the Server may offer connection
  settings for metrics using own_metrics field.
- If ReportsOwnLogs capability bit is set the Server may offer connection
  settings for logs using own_logs field.
- If AcceptsOpAMPConnectionSettings capability bit is set the Server may offer
  connection settings for OpAMP connection using opamp field.
- If AcceptsOtherConnectionSettings capability bit is set the Server may offer
  connection settings for other destinations using other_connections field.

In addition the ReportsConnectionSettingsStatus capability is used by the Agent
to indicate that it is able to report to the server if the offered connection
settings are applied as expected.

Depending on which connection settings are offered the sequence of operations is
slightly different. The handling of connection settings for own telemetry is
described in [Own Telemetry Reporting](#own-telemetry-reporting). The handling
of connection settings for "other" destinations is described in
[Connection Settings for "Other" Destinations](#connection-settings-for-other-destinations).
The handling of OpAMP connection settings is described below.

It is also possible for the Agent to make a request to the Server to initiate the
connection settings creation. This process is described in
[Agent-initiated CSR Flow](#agent-initiated-csr-flow) section.

#### OpAMP Connection Setting Offer Flow

Here is how the server-initiated OpAMP connection settings change happens:

```
                   Client                                 Server

                     │                                       │    Initiate
                     │    Connect                            │    Settings
                     ├──────────────────────────────────────►│     Change
                     │                 ...                   │        │
                     │                                       │◄───────┘
                     │                                       │          ┌───────────┐
                     │                                       ├─────────►│           │
                     │                                       │ Generate │Credentials│
┌───────────┐        │ServerToAgent{ConnectionSettingsOffers}│ and Save │   Store   │
│           │◄───────┤◄──────────────────────────────────────┤◄─────────┤           │
│Credentials│ Save   │                                       │          └───────────┘
│   Store   │        │             Disconnect                │
│           ├───────►├──────────────────────────────────────►│
└───────────┘        │                                       │
                     │    Connect, New settings              │          ┌───────────┐
                     ├──────────────────────────────────────►├─────────►│           │
                     │                                       │ Delete   │Credentials│
┌───────────┐        │    Connection established             │ old      │   Store   │
│           │◄───────┤◄─────────────────────────────────────►│◄─────────┤           │
│Credentials│Delete  │                                       │          └───────────┘
│   Store   │old     │                                       │
│           ├───────►│                                       │
└───────────┘        │                                       │

```

1. Server generates new connection settings and saves it in Server's credentials
   store, associating the new settings with the Agent instance UID.
2. Server sends the ServerToAgent message that includes
   [ConnectionSettingsOffers](#connectionsettingsoffers-message) message. The
   [opamp](#connectionsettingsoffersopamp) field contains the new
   [OpAMPConnectionSettings](#opampconnectionsettings) offered.
3. Client receives the settings offer and saves the updated
   connection settings in the local store, marking it as "candidate" (if Client
   crashes it will retry "candidate" validation steps 5-9).
4. Client disconnects from the Server. If the client has the
   ReportsConnectionSettingsStatus capability, the Client SHOULD indicate that
   it is applying the new connection settings.
5. Client connects to the Server, using the new settings.
6. Connection is successfully established, any TLS verifications required are
   passed and the Server indicates a successful authorization.
7. Server deletes the old connection settings for this Agent (using Agent
   instance UID) from its credentials store.
8. Client deletes the old settings from its credentials store and marks the new
   connection settings as "valid". If the client has the
   ReportsConnectionSettingsStatus capability the client MUST report that the
   connection settings have been applied.
9. If step 6 fails the Client deletes the new settings and reverts to the old
   settings and reconnects. If the Client has the
   ReportsConnectionSettingsStatus capability the client MUST report that the
   connection settings have failed.

Note: Clients which are unable to persist new connection settings and have access
only to ephemeral storage SHOULD reject certificate offers otherwise they risk
losing access after restarting and losing the offered certificate.

#### Trust On First Use

OpAMP Clients that want to use TLS with a client certificate but do not initially have
a certificate can use the Trust On First Use (TOFU) flow. The sequence is the
following:

* Client connects to the Server using regular TLS (validating Server's identity)
  but without a client certificate. Client sends the Agent's Status Report so that it can
  be identified.
* The Server accepts the connection and status and awaits for an approval to
  generate a client certificate for the OpAMP Client.
* Server either waits for a manual approval by a human or automatically approves
  all TOFU requests if the Server is configured to do so (can be a Server-side
  option).
* Once approved the flow is essentially identical to
  [OpAMP Connection Setting Offer Flow](#opamp-connection-setting-offer-flow)
  steps, except that there is no old client certificate to delete.

TOFU flow allows to bootstrap a secure environment without the need to perform
Client-side installation of certificates.

Exact same TOFU approach can be also used for OpAMP Clients that don't have the
necessary authorization headers to access the Server. The Server can detect such
access and upon approval send the authorization headers to the Client.

#### Registration On First Use

In some use cases it may be desirable to equip newly installed Agents with an
initial connection settings that are good for the first use, but generate a new
set of connection credentials after the first connection is established.

This can be achieved very similarly to how the TOFU flow works. The only
difference is that the first connection will be properly authenticated, but the
Server will immediately generate and offer new connection settings to the Agent.
The Client will then persist the setting and will use them for all subsequent
operations.

This allows deploying a large number of Agents using one pre-defined set of
connection credentials (authorization headers, certificates, etc), but
immediately after successful connection each Agent will acquire their own unique
connection credentials. This way individual Agent's credentials may be revoked
without disrupting the access to all other Agents.

#### Agent-initiated CSR Flow

Status: [Development]

This is an Agent-initiated flow that allows the Client to send a Certificate
Signing Request (CSR) to the Server and obtain a self-signed or CA-signed client
certificate that the Client can use for subsequent OpAMP connections.

This flow is currently only supported for OpAMP connections. It is not possible
for the Agent to send a CSR request for own telemetry connections or for other
connection types.

```
                   Client                                 Server

                     │ (1)           Connect                 │
                     ├──────────────────────────────────────►│
                     │                 ...                   │
┌───────────┐        │                                       │          ┌───────────┐
│ Generate  │  (2)   │ (3)     AgentToServer{CSR}            │(4)       │           │
│ Keypair   ├───────►├──────────────────────────────────────►├─────────►│  Approve  │
│ and CSR   │        │                 ...                   │          │           │
└───────────┘        │                                       │          └─────┬─────┘
                     │                                       │                │(5)
                     │                                       │                │
                     │                                       │                ▼
                     │                                       │          ┌───────────┐
┌───────────┐        │ServerToAgent{ConnectionSettingsOffers}│ (7)      │Create     │
│           │◄───────┤◄──────────────────────────────────────┤◄─────────┤Certificate│
│Credentials│ Save   │                                       │          │    (6)    │
│   Store   │        │             Disconnect                │          └───────────┘
│           ├───────►├──────────────────────────────────────►│
└───────────┘        │                                       │
                     │    Connect, New settings              │          ┌───────────┐
                     ├──────────────────────────────────────►├─────────►│           │
                     │                                       │ Delete   │Credentials│
┌───────────┐        │    Connection established             │ old      │   Store   │
│           │◄───────┤◄─────────────────────────────────────►│◄─────────┤           │
│Credentials│Delete  │                                       │          └───────────┘
│   Store   │old (8) │                                       │
│           ├───────►│                                       │
└───────────┘        │                                       │
```

The sequence is the following:

- (1) The Client connects to the Server. The Client SHOULD use regular TLS and validate
  the Server's identity. The Agent may also use a bootstrap client certificate that is
  already trusted by the Server. (Note: the distribution and installation method of
  the bootstrap certificate is not part of this specification).
- (2) The Agent generates a keypair and a Certificate Signing Request (CSR). The CSR
  contains the information that client wants to use for subsequent authentication.
- (3) The Client sends an AgentToServer message containing the CSR.
- (4) If a bootstrap certificate is provided the Server validates it (e.g. with a
  trusted CA) and awaits for an approval to generate a client certificate for the Agent.
- (5) The Server either waits for a manual approval by a human or automatically
  approves all TOFU requests if the Server is configured to do so (can be a
  Server-side option).
- (6) Once approved, the Server creates a client certificate. The Server does this
  either by issuing a self-signed certificate (acting as a local CA) or proxies the
  CSR to a CA and obtains a client certificate from the CA.
- (7) After obtaining the client certificate from the CA the flow is essentially
  identical to [OpAMP Connection Setting Offer Flow](#opamp-connection-setting-offer-flow)
  steps, starting by offering the connection settings that carry the created client
  certificate. The OpAMPConnectionSettings.certificate message will have the
  cert field set to the client certificate. If a CA is used the ca_cert field
  will be set to the CA's certificate. The private_key field will not be set, since in
  this flow the Agent possesses the private key and the Server does not possess it.
- (8) Upon successfully verifying of the offered new client certificate,
  the Agent removes the bootstrap certificate if one was used and uses the new
  certificate for future connections.

When sending OpAMPConnectionSettings to the Agent the Server MAY include fields
other than `certificate`, thus enabling the Server to replace Agent's certificate,
connection headers and other settings at once.

If any of the steps 4-6 fails the Server MUST respond to the Agent with a
[ServerErrorResponse](#servererrorresponse-message) with the `type` field set
to `ServerErrorResponseType_BadRequest`.

The exact same flow may be used by the Agent to re-request a new certificate anytime.
For example the Agent may do it when the current certificate expiration date approaches.

##### Using instance_uid in the CSR

Status: [Development]

The implementation may choose to use Agent's instance_uid as one of the CSR fields
(or part of the field) and the Server may in such implementations verify that the
connecting Agent's instance_uid in the payloads matches the certificate's content.
This prevents Agents impersonating other Agents.

When the Server receives a CSR containing the instance_uid in CSR fields the Server
MUST verify that the [instance_uid](#agenttoserverinstance_uid) field in AgentToServer
message matches the instance_uid in the CSR fields. This enforces that the Agent
may only ask for a certificate for itself.

If the instance_uid is part of the CSR and the issued client certificate then any change
of the instance_uid requires re-generation of the client certificate. Such change is
possible for example if the Server instructs the Agent to use a new instance_uid
via the [new_instance_uid](#servertoagentagent_identification) field.

When instructed by the Server to change its instance_uid the Agent must also repeat the
[Agent-initiated CSR Flow](#agent-initiated-csr-flow) this time using the new
instance_uid as one of the CSR fields. The Server must then be ready to receive a CSR
while the Agent is still using the old certificate that contains the old
instance_uid.

In other words: an incoming CSR may ask for a new certificate that references an
instance_uid that is different from the instance_uid that is referenced in the
client certificate that is used by that same incoming connection. This is a valid
situation and must not be rejected by the Server. In this situation the Server may
also look at the instance_uid in the connecting client certificate and compare it
to the old known instance_uid to confirm that the Agent is indeed performing
an instance_uid change as instructed by the Server.

#### Revoking Access

Since the Server knows what access headers and a client certificate the Client
uses, the Server can revoke access to individual Agents by marking the
corresponding connection settings as "revoked" and disconnecting the Client.
Subsequent connections using the revoked credentials can be rejected by the
Server essentially prohibiting the Client to access the Server.

Since the Server has control over the connection settings of all 3 destination
types of the Agent (because it can offer the connection settings) this
revocation may be performed for either of the 3 types of the destinations,
provided that the Server previously offered and the Agent accepted the
particular type of destination.

For own telemetry and "other" destinations the Server MUST also communicate the
revocation fact to the corresponding destinations so that they can begin
rejecting access to connections that use the revoked credentials.

#### Certificate Generation

Client certificates that the Server generates may be self-signed, signed by a
private Certificate Authority or signed by a public Certificate Authority. The
Server is responsible for generating client certificates such that they are
trusted by the destination the certificate is intended for. This requires that
either the destinations remember and trust the individual self-signed client
certificate directly or they trust the Certificate Authority that is used for
signing the client certificate so that the trust chain can be verified.

How exactly the client certificates are generated is outside the scope of the
OpAMP specification.

#### Connection Settings for "Other" Destinations

TBD

#### ConnectionSettingsRequest Message

Status: [Development]

ConnectionSettingsRequest is a request from the Agent to the Server to create
and respond with an offer of connection settings for the Agent.

```protobuf
message ConnectionSettingsRequest {
    OpAMPConnectionSettingsRequest opamp = 1;
}
```

The `opamp` field is set to indicate a request for OpAMP connection settings.
If this field is unset then the ConnectionSettingsRequest message is empty and is
not actionable for the Server.

#### OpAMPConnectionSettingsRequest Message

Status: [Development]

OpAMPConnectionSettingsRequest is a request for the Server to produce
an OpAMPConnectionSettings in its response.

Used for [Agent-initiated CSR Flow](#agent-initiated-csr-flow).

```protobuf
message OpAMPConnectionSettingsRequest {
    CertificateRequest certificate_request = 1;
}
```

certificate_request is set when the Agent requests the Server to create a client
certificate. This field is required.

#### CertificateRequest Message

Status: [Development]

```protobuf
message CertificateRequest {
    bytes csr = 1;
}
```

The `csr` field is the PEM-encoded Client Certificate Signing Request (CSR), signed by
client's private key.

The Server SHOULD validate the request and SHOULD respond with a
OpAMPConnectionSettings where the certificate.cert contains the issued
certificate.

#### ConnectionSettingsOffers Message

ConnectionSettingsOffers message describes connection settings for the Agent to
use:

```protobuf
message ConnectionSettingsOffers {
    bytes hash = 1;
    OpAMPConnectionSettings opamp = 2;
    TelemetryConnectionSettings own_metrics = 3;
    TelemetryConnectionSettings own_traces = 4;
    TelemetryConnectionSettings own_logs = 5;
    map<string,OtherConnectionSettings> other_connections = 6;
}
```

##### ConnectionSettingsOffers.hash

Hash of all settings, including settings that may be omitted from this message
because they are unchanged.

##### ConnectionSettingsOffers.opamp

Settings to connect to the OpAMP Server. If this field is not set then the Client
should assume that the settings are unchanged and should continue using existing
settings. The Client MUST verify the offered connection settings by actually
connecting before accepting the setting to ensure it does not lose access to
the OpAMP Server due to invalid settings.

##### ConnectionSettingsOffers.own_metrics

Settings to connect to an OTLP metrics backend to send Agent's own metrics to.
If this field is not set then the Agent should assume that the settings are
unchanged.

##### ConnectionSettingsOffers.own_traces

Settings to connect to an OTLP traces backend to send Agent's own traces to. If
this field is not set then the Agent should assume that the settings are
unchanged.

##### ConnectionSettingsOffers.own_logs

Settings to connect to an OTLP logs backend to send Agent's own logs to. If
this field is not set then the Agent should assume that the settings are
unchanged.

##### ConnectionSettingsOffers.other_connections

Another set of connection settings, with a string name associated with each. How
the Agent uses these is Agent-specific. Typically the name represents the name
of the destination to connect to (as it is known to the Agent). If this field is
not set then the Agent should assume that the other_connections settings are
unchanged.

#### OpAMPConnectionSettings

The OpAMPConnectionSettings message is a collection of fields which comprise an
offer from the Server to the OpAMP Client to use the specified settings for OpAMP
connection.

```protobuf
message OpAMPConnectionSettings {
    string destination_endpoint = 1;
    Headers headers = 2;
    TLSCertificate certificate = 3;
    uint64 heartbeat_interval_seconds = 4;
    TLSConnectionSettings tls = 5;
    ProxyConnectionSettings proxy = 6;
}
```

##### OpAMPConnectionSettings.destination_endpoint

OpAMP Server URL This MUST be a WebSocket or HTTP URL and MUST be non-empty, for
example, `wss://example.com:4318/v1/opamp`.

##### OpAMPConnectionSettings.headers

Optional headers to use when connecting. Typically used to set access tokens or
other authorization headers. For HTTP-based protocols the Client should
set these in the request headers.
For example:
key="Authorization", Value="Basic YWxhZGRpbjpvcGVuc2VzYW1l".

##### OpAMPConnectionSettings.certificate

The Client should use the offered certificate to connect to the destination
from now on. If the Client is able to validate and connect using the offered
certificate the Client SHOULD forget any previous client certificates
for this connection.
This field is optional: if omitted the client SHOULD NOT use a client-side certificate.
This field can be used to perform a client certificate revocation/rotation.

##### OpAMPConnectionSettings.heartbeat_interval_seconds

Status: [Development]

If the ReportsHeartbeat capability is true, the Client MUST use the offered heartbeat
interval to periodically send an AgentToServer message. If the capability is true
and the Server sets heartbeat_interval_seconds to 0, Agent heartbeats should be disabled.
At a minimum the `AgentToServer.instance_uid` field MUST be set in the heartbeats.
An HTTP-based client MUST use the heartbeat interval as its polling interval.

Any AgentToServer message where instance_uid field is set is considered a
valid heartbeat. Note that it is not necessary to send a separate AgentToServer
message just for heartbeating purposes if another AgentToServer message
containing other data was just sent. The Agent must count heartbeating interval
from the last AgentToServer message sent.

A heartbeat is used to keep a connection active and inform the server that
the Agent is still alive and active. A server could use the heartbeat to make decisions about
the liveness of the connected Agent.

The flow for negotiating a heartbeat is described as so:

```
┌──────────┐                       ┌──────────┐
│          │ (1) Connect           │          │
│          ├──────────────────────►│          │
│          │                       │          │
│          │ (2) Set Heartbeat     │          │
│          │◄──────────────────────┤          │
│          │     Interval          │          │
│          │                       │          │
│  Agent   │ (3) Send Heartbeat    │  Server  │
│          ├──────────────────────►│          │
│          │                       │          │
│          │  ... heartbeat        │          │
│          │      interval         │          │
│          │                       │          │
│          │ (4) Send Heartbeat    │          │
│          ├──────────────────────►│          │
│          │                       │          │
└──────────┘                       └──────────┘
```

1. The agent connects to the server and optionally sets the ReportsHeartbeat capability. If the Agent does not set this capability, the Server should not expect to receive heartbeats.
2. If the Agent sets the ReportsHeartbeat capability, the server MAY respond by setting an interval in the heartbeat_interval_seconds field within the OpAMPConnectionSettings message. The value can either be the desired interval, or `0`, indicating that the client should not send heartbeats. 30s is the recommended default interval.
3. If the Agent sets the ReportsHeartbeat capability AND the server hasn't disabled heartbeats, the Agent MUST send a heartbeat message every period, specified by the interval set by the server or using the agent's configured heartbeat interval.
4. The Agent will continue to send heartbeats on its configured interval while alive.

The Agent can decide not to send heartbeats by not setting the ReportsHeartbeat capability. The Server can decide to not receive heartbeats by responding with a value of `0` seconds in the OpAMPConnectionSettings.heartbeat_interval_seconds field.

##### OpAMPConnectionSettings.tls

Status: [Development]

Optional OpAMP specific TLS settings.

##### OpAMPConnectionSettings.proxy

Status: [Development]

Optional OpAMP specific proxy settings.

#### TelemetryConnectionSettings

The TelemetryConnectionSettings message is a collection of fields which comprise an
offer from the Server to the Agent to use the specified settings for a network
connection to report own telemetry.

```protobuf
message TelemetryConnectionSettings {
    string destination_endpoint = 1;
    Headers headers = 2;
    TLSCertificate certificate = 3;
    TLSConnectionSettings tls = 4;
    ProxyConnectionSettings proxy = 5;
}
```

##### TelemetryConnectionSettings.destination_endpoint

The value MUST be a full URL an OTLP/HTTP/Protobuf receiver with path. Schema
SHOULD begin with `https://`, for example, `https://example.com:4318/v1/metrics`.
The Agent MAY refuse to send the telemetry if the URL begins with `http://`.

##### TelemetryConnectionSettings.headers

Optional headers to use when connecting. Typically used to set access tokens or
other authorization headers. For HTTP-based protocols the Agent should
set these in the request headers.
For example:
key="Authorization", Value="Basic YWxhZGRpbjpvcGVuc2VzYW1l".

##### TelemetryConnectionSettings.certificate

The Agent should use the offered certificate to connect to the destination
from now on. If the Agent is able to validate and connect using the offered
certificate the Agent SHOULD forget any previous client certificates
for this connection.
This field is optional: if omitted the client SHOULD NOT use a client-side certificate.
This field can be used to perform a client certificate revocation/rotation.

##### TelemetryConnectionSettings.tls

Status: [Development]

Optional telemetry specific TLS settings.

##### TelemetryConnectionSettings.proxy

Status: [Development]

Optional telemetry specific proxy settings.

#### OtherConnectionSettings

The OtherConnectionSettings message is a collection of fields which comprise an
offer from the Server to the Agent to use the specified settings for a network
connection. It is not required that all fields in this message are specified.
The Server may specify only some of the fields, in which case it means that
the Server offers the Agent to change only those fields, while keeping the
rest of the fields unchanged.

For example the Server may send a ConnectionSettings message with only the
certificate field set, while all other fields are unset. This means that
the Server wants the Agent to use a new certificate and continue sending to
the destination it is currently sending using the current header and other
settings.

For fields which reference other messages the field is considered unset
when the reference is unset.

For primitive field (string) we rely on the "flags" to describe that the
field is not set (this is done to overcome the limitation of old `protoc`
compilers don't generate methods that allow to check for the presence of
the field.

```protobuf
message OtherConnectionSettings {
    string destination_endpoint = 1;
    Headers headers = 2;
    TLSCertificate certificate = 3;
    map<string, string> other_settings = 4;
    TLSConnectionSettings tls = 5;
    ProxyConnectionSettings proxy = 6;
}
```

##### OtherConnectionSettings.destination_endpoint

A URL, host:port or some other destination specifier.

##### OtherConnectionSettings.headers

Optional headers to use when connecting. Typically used to set access tokens or
other authorization headers. For HTTP-based protocols the Agent should
set these in the request headers.
For example:
key="Authorization", Value="Basic YWxhZGRpbjpvcGVuc2VzYW1l".

##### OtherConnectionSettings.certificate

The Agent should use the offered certificate to connect to the destination
from now on. If the Agent is able to validate and connect using the offered
certificate the Agent SHOULD forget any previous client certificates
for this connection.
This field is optional: if omitted the client SHOULD NOT use a client-side certificate.
This field can be used to perform a client certificate revocation/rotation.

##### OtherConnectionSettings.other_settings

Other connection settings. These are Agent-specific and are up to the Agent
interpret.

##### OtherConnectionSettings.tls

Status: [Development]

Optional connection specific TLS settings.

##### OtherConnectionSettings.proxy

Status: [Development]

Optional connection specific proxy settings.

#### TLSConnectionSettings Message

Status: [Development]

The message carries optional TLS settings that are used to configure a client's
connection. If the Agent is able to validate the connection settings, the Agent
SHOULD forget any previous TLS settings. If this message is not included, the
client SHOULD assume the settings are unchanged and continue using existing
settings.

```protobuf
message TLSConnectionSettings {
  string ca_pem_contents = 1;
  bool include_system_ca_certs_pool = 2;
  bool insecure_skip_verify = 3;
  string min_version = 4;
  string max_version = 5;
  repeated string cipher_suites = 6;
}
```

##### TLSConnectionSettings.ca_pem_contents

The ca_pem_contents attribute may be used to provide the CA's public
certificate as part of the TLS configuration.

##### TLSConnectionSettings.include_system_ca_pool

The include_system_ca_pool asks the Agent to use the system's default CA pool
when building TLS configuration.

##### TLSConnectionSettings.insecure_skip_verify

This setting disables all TLS verification for connections.

##### TLSConnectionSettings.min_version

This sets the minimum supported TLS version the client will use. For example:
`1.2`, `TLSv1.2`.

##### TLSConnectionSettings.max_version

This sets the maximum supported TLS version the client will use. For example:
`1.2`, `TLSv1.2`.

##### TLSConnectionSettings.ciper_suites

This sets the supported cipher suites that may be used by the connection. For
example: `TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA`.

#### ProxyConnectionSettings Message

Status: [Development]

The message carries optional proxy settings that are used to configure a
client's connection. If the Agent is able to validate the connection settings,
the Agent SHOULD forget any previous proxy settings. If this message is not
included, client SHOULD assume the settings are unchanged and continue using
existing settings.

If the proxy requires an mTLS certificate, or any TLS settings the Client
SHOULD use the associated connection's settings.

```protobuf
message ProxyConnectionSettings {
    string url = 1;
    Headers connect_headers = 2;
}
```

##### ProxyConnectionSettings.url

The url MUST be non-empty, for example: `https://example.com:8443` or
`127.0.0.1:8443`.

##### ProxyConnectionSettings.connect_headers

Option headers the Client should set for HTTP based proxies initial CONNECT
request. Other proxy types, such as SOCKS5 may ignore these headers.
For example:
key="Authorization", Value="Basic YWxhZGRpbjpvcGVuc2VzYW1l".

#### Headers Message

```protobuf
message Headers {
    repeated Header headers = 1;
}
message Header {
    string key = 1;
    string value = 2;
}
```

#### TLSCertificate Message

The message carries a TLS certificate that can be used as a client-side
certificate.

The (cert,private_key) pair should be issued and signed by a Certificate
Authority (CA) that the destination Server recognizes.

Alternatively the certificate may be self-signed, assuming the Server can verify
the certificate. In this case the ca_cert field can be omitted.

```protobuf
message TLSCertificate {
    bytes cert = 1;
    bytes private_key = 2;
    bytes ca_cert = 3;
}
```

##### TLSCertificate.cert

PEM-encoded certificate. Required.

##### TLSCertificate.private_key

PEM-encoded private key of the certificate. Required.

##### TLSCertificate.ca_cert

PEM-encoded certificate of the signing CA. Optional, MUST be
specified if the certificate is CA-signed. Can be stored by intermediary
TLS-terminating proxies in order to verify the connecting client's certificate
in the future.

### Own Telemetry Reporting

Status: [Beta]

Own Telemetry Reporting is an optional capability of OpAMP protocol. The Server
can offer to the Agent a destination to which the Agent can send its own
telemetry (metrics, traces or logs). If the Agent is capable of producing
telemetry and wishes to do so then it should sends its telemetry to the offered
destination using OTLP/HTTP protocol:

```
            ┌────────────┬────────┐           ┌─────────┐
            │            │ OpAMP  │  OpAMP    │ OpAMP   │
            │            │        ├──────────►│         │
            │            │ Client │           │ Server  │
            │            └────────┤           └─────────┘
            │   Agent             │
            │            ┌────────┤           ┌─────────┐
            │            │OTLP    │ OTLP/HTTP │ OTLP    │
            │            │        ├──────────►│ Metric  │
            │            │Exporter│           │ Backend │
            └────────────┴────────┘           └─────────┘
```

The Server makes the offer by sending a [ServerToAgent](#servertoagent-message)
message with a populated [connection_settings](#servertoagentconnection_settings) field
that contains one or more of the own_metrics, own_traces, own_logs fields set. Each
of these fields describes a destination, which can receive telemetry using OTLP
protocol.

The Server SHOULD populate the [connection_settings](#servertoagentconnection_settings)
field when it sends the first ServerToAgent message to the particular Agent (normally
in response to the first status report from the Client), unless there is no OTLP
backend that can be used. The Server SHOULD also populate the field on
subsequent ServerToAgent if the destination has changed. If the destination is
unchanged the connection_settings field SHOULD NOT be set. When the Agent
receives a ServerToAgent with an unset connection_settings field the Agent SHOULD
continue sending its telemetry to the previously offered destination. If the
Agent has the ReportsConnectionSettingsStatus capability it SHOULD set the
[connection_settings_status](#connectionsettingsstatus-message) accordingly
when new settings are received.

The Agent SHOULD periodically report its metrics to the destination offered in the
[own_metrics](#connectionsettingsoffersown_metrics) field. The recommended reporting
interval is 10 seconds. Here is the diagram that shows the operation sequence:

```
       Agent    Client                                Server
                                                            Metric
        │         │                                      │  Backend
        │         │ServerToAgent{ConnectionSettingsOffer}│
        ┌─────────│◄─────────────────────────────────────┤    │
        │                                                │    │
        ▼                                                     │
    ┌────────┐                                                │
    │Collect │                     OTLP Metrics               │ ──┐
    │Own     ├───────────────────────────────────────────────►│   │
    │Metrics │                                                │   │
    └────────┘                         ...                    .   │ Repeats
        │                                                         │
    ┌────────┐                                                │   │ Periodically
    │Collect │                     OTLP Metrics               │   │
    │Own     ├───────────────────────────────────────────────►│   │
    │Metrics │                                                │ ──┘
    └────────┘                                                │
```

The Agent SHOULD report metrics of the Agent process (or processes) and any
custom metrics that describe the Agent state. Reported process metrics MUST
follow the OpenTelemetry
[conventions for processes](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/system/process-metrics.md).

Similarly, the Agent SHOULD report its traces to the destination offered in the
[own_traces](#connectionsettingsoffersown_traces) field and logs to the destination
offered in the [own_logs](#connectionsettingsoffersown_logs) field.

All attributes specified in the
[identifying_attributes](#agentdescriptionidentifying_attributes) field in
AgentDescription message SHOULD be also specified in the Resource of the reported OTLP
telemetry.

Attributes specified in the
[non_identifying_attributes](#agentdescriptionnon_identifying_attributes) field in
AgentDescription message may be also specified in the Resource of the reported
OTLP telemetry, in which case they SHOULD have exactly the same values.

### Configuration

Agent configuration is an optional capability of OpAMP protocol. Remote
configuration capability can be disabled if necessary (for example when using
existing configuration capabilities of an orchestration system such as
Kubernetes).

The Server can offer a Remote Configuration to the Agent by setting the
[remote_config](#servertoagentremote_config) field in the ServerToAgent message. Since the
ServerToAgent message is normally sent by the Server in response to a status
report the Server has the Agent's description and may tailor the configuration
it offers to the specific Agent if necessary.

The OpAMP Client MUST set the AcceptsRemoteConfig bit of AgentToServer.capabilities if
the Agent is capable of accepting remote configuration. If the bit is not set
the Server MUST not offer a remote configuration to the Agent.

The Agent's actual configuration that it uses for running may be different from
the Remote Configuration that is offered by the Server. This actual
configuration is called the Effective Configuration of the Agent. The Effective
Configuration is typically formed by the Agent after merging the Remote
Configuration with other inputs available to the Agent, e.g. a locally available
configuration.

Once the Effective Configuration is formed the Agent uses it for its operation
and the Client will also report the Effective Configuration to the OpAMP Server via the
[effective_config](#agenttoservereffective_config) field of status report. The Server
typically allows the end user to see the effective configuration alongside other
data reported in the status reported by the Client.

The Client MUST set the ReportsEffectiveConfig bit of AgentToServer.capabilities
if the Agent is capable of reporting effective configuration. If the bit is not
set the Server should not expect the AgentToServer.effective_config field to be
set.

Here is the typical configuration sequence diagram:

```
     Agent       Client                             Server

       │           │ AgentToServer{}                   │   ┌─────────┐
       │           ├──────────────────────────────────►├──►│ Process │
       │           │                                   │   │ Status  │
Local  │    Remote │                                   │   │ and     │
Config │    Config │ ServerToAgent{AgentRemoteConfig}  │   │ Fetch   │
  │    │  ┌────────┤◄──────────────────────────────────┤◄──┤ Config  │
  ▼    │  ▼        │                                   │   └─────────┘
 ┌─────────┐       │                                   │
 │ Config  │       │                                   │
 │ Merger  │       │                                   │
 └─────┬───┘       │                                   │
       │           │                                   │
       │Effective  │                                   │
       │Config     │ AgentToServer{}                   │
       └──────────►├──────────────────────────────────►│
                   │                                   │
                   │                                   │
```

EffectiveConfig and RemoteConfigStatus fields are included in the AgentToServer
message if the fields have changed.

Note: the Client SHOULD NOT send AgentToServer message if the Effective Configuration
or other fields that are reported via AgentToServer message are unchanged.
If the Client does not follow this rule the operation may result in an infinite loop of
messages sent back and forth between the Client and the Server.

The Server may also initiate sending of a remote configuration on its own,
without waiting for a status report from the Client. This can be used to
re-configure an Agent that is connected but which has nothing new to report. The
sequence diagram in this case looks like this:

```
    Agent      Client                             Server

       │           │                                   │
       │           │                                   │
       │           │                                   │   ┌────────┐
Local  │    Remote │                                   │   │Initiate│
Config │    Config │  ServerToAgent{AgentRemoteConfig} │   │and     │
    │  │  ┌────────┤◄──────────────────────────────────┤◄──┤Send    │
    ▼  │  ▼        │                                   │   │Config  │
  ┌─────────┐      │                                   │   └────────┘
  │ Config  │      │                                   │
  │ Merger  │      │                                   │
  └────┬────┘      │                                   │
       │           │                                   │
       │Effective  │                                   │
       │Config     │ AgentToServer{}                   │
       └──────────►├──────────────────────────────────►│
                   │                                   │
                   │                                   │
```

The Agent may ignore the Remote Configuration offer if it does not want its
configuration to be remotely controlled by the Server.

#### Configuration Files

The configuration of the Agent is a collection of named configuration files
(this applies both to the Remote Configuration and to the Effective
Configuration).

The file names MUST be unique within the collection. It is possible that the
Remote and Local Configuration MAY contain a file with the same name but with a
different content. How these files are merged to form an Effective Configuration
is Agent type-specific and is not part of the OpAMP protocol.

If there is only one configuration file in the collection then the file name MAY
be empty.

The collection of configuration files is represented using a AgentConfigMap
message:

```protobuf
message AgentConfigMap {
  map<string, AgentConfigFile> config_map = 1;
}
```

The config_map field of the AgentConfigSet message is a map of configuration
files, where keys are file names.

For Agents that use a single config file the config_map field SHOULD contain a
single entry and the key MAY be an empty string.

The AgentConfigFile message represents one configuration file and has the
following structure:

```protobuf
message AgentConfigFile {
  bytes body = 1;
  string content_type = 2;
}
```

The body field contains the raw bytes of the configuration file. The content,
format and encoding of the raw bytes is Agent type-specific and is outside the
concerns of OpAMP protocol.

content_type is an optional field. It is a MIME Content-Type that describes
what's contained in the body field, for example "text/yaml". The content_type
reported in the Effective Configuration in the Agent's status report may be used
for example by the Server to visualize the reported configuration nicely in a
UI.

#### Security Considerations

Remote Configuration is a potentially dangerous functionality that may be
exploited by malicious actors. For example if the Agent is capable of collecting
local files and sending over the network then a compromised OpAMP Server may
offer a malicious remote configuration to the Agent and compel the Agent to
collect a sensitive local file and send to a specific network destination.

See Security section for [general recommendations](#general-recommendations) and
recommendations specifically for
[remote reconfiguration](#configuration-restrictions) capabilities.

#### AgentRemoteConfig Message

The message has the following structure:

```protobuf
message AgentRemoteConfig {
  AgentConfigMap config = 1;
  bytes config_hash = 2;
}
```

### Packages

Status: [Beta]

Each Agent is composed of one or more packages. A package has a name and content stored
in a file. The content of the file, functionality provided by the packages, how they are
stored and used by the Agent side is Agent type-specific and is outside the concerns of
the OpAMP protocol.

There are two types of packages: top-level and sub-packages.

There is normally only one top-level package, which implements the primary
functionality of the Agent. When there is only one top-level package it may have an
empty name.

Sub-packages are also known as addons or plugins. The sub-packages can be installed at
the Agent for added functionality (hence the name addons).

The Agent may have one or more packages installed. Each package has a name. The
Agent cannot have more than one package of the particular name installed.

Different package may have files with the same name, file names are not globally
unique, they are only unique within the scope of a particular package.

Package may be provided and installed locally (e.g. by a local user). The packages
may be also offered to the Agent remotely by the Server, in which case the Agent
may download and install the packages.

To offer packages to the Agent the Server sets the
[packages_available](#servertoagentpackages_available) field in the ServerToAgent message
that is sent either in response to a status report form the Agent or by Server's
initiative if the Server wants to push packages to the Agent.

The [PackagesAvailable](#packagesavailable-message) message describes the packages
that are available on the Server for this Agent. For each package the message
describes the file that has the package's content and provides the URL from which
the file can be downloaded using an HTTP GET request. The URLs point to package
files on a Download Server (which may be on the same host as the OpAMP Server or
a different host).

The protocol supports only a single downloadable file per package. If the Agent's
packages conceptually are composed of multiple files then the Agent and Server can
agree to store the files in any file format that allows storing multiple files
in a single file, e.g. a zip or tar file. After downloading the single package
file the Agent may extract the files contained in it. How exactly this is done
is Agent specific and is beyond the scope of the protocol.

The Server is allowed to make a package offer only if the OpAMP Client indicated that
the Agent can accept packages via AcceptsPackages bit of AgentToServer.capabilities field.

#### Downloading Packages

After receiving the [PackagesAvailable](#packagesavailable-message) message the
Agent SHOULD follow this download procedure:

##### Step 1

Compare the aggregate hash of all packages it has with the aggregate hash offered
by the Server in the all_packages_hash field.

If the aggregate hash is the same then consider the download procedure done,
since it means all packages on the Agent are the same as offered by the Server.
Otherwise, go to Step 2.

##### Step 2

For each package offered by the Server the Agent SHOULD check if it should
download the particular package:

* If the Agent does not have a package with the specified name then it SHOULD
  download the package. See Step 3 on how to download each package file.
* If the Agent has the package the Agent SHOULD compare the hash of the package that
  the Agent has with the hash of the package offered by the Server in the
  [hash](#packageavailablehash) field in the [PackageAvailable](#packageavailable-message)
  message.
  If the hashes are the same then the package is the same and processing for this
  package is done, proceed to the next package. If the hashes are different then
  check the package file as described in Step 3.

Finally, if the Agent has any packages that are not offered by the Server the
packages SHOULD be deleted by the Agent.

##### Step 3

For the file of the package offered by the Server the Agent SHOULD check if it
should download the file:

* If the Agent does not have a file with the specified name then it SHOULD
  download the file.
* If the Agent has the file then the Agent SHOULD compare the hash of the file
  it has locally with the [hash](#downloadablefilecontent_hash) field in the
  [DownloadableFile](#downloadablefile-message) message. If hashes are the same
  the processing of this file is done. Otherwise, the offered file is different
  and the file SHOULD be downloaded from the location specified in the
  [download_url](#downloadablefiledownload_url) field of the
  [DownloadableFile](#downloadablefile-message) message. The Agent SHOULD use an
  HTTP GET message to download the file. The Agent SHOULD include the HTTP
  headers provided in the [headers](#downloadablefileheaders) field for the GET
  request.

The procedure outlined above allows the Agent to efficiently download only new
or changed packages and only download new or changed files.

After downloading the packages the Agent can perform any additional processing
that is Agent type-specific (e.g. "install" or "activate" the packages in any way
that is specific to the Agent).

#### Package Status Reporting

During the downloading and installation process the Agent MAY periodically
report the status of the process. To do this the OpAMP Client SHOULD send an
[AgentToServer](#agenttoserver-message) message and set the
[package_statuses](#agenttoserverpackage_statuses) field accordingly.

Once the downloading and installation of all packages is done (succeeded or
failed) the Client SHOULD report the status of all packages to the Server.

Here is the typical sequence diagram for the package downloading and status
reporting process:

```
    Download        Agent/Client                          OpAMP
     Server                                              Server
       │                 │                                  │
       │                 │  ServerToAgent{PackagesAvailable}│
       │                 │◄─────────────────────────────────┤
       │   HTTP GET      │                                  │
       │◄────────────────┤                                  │
       │ Download file #1│                                  │
       ├────────────────►│                                  │
       │                 │ AgentToServer{PackageStatuses}   │
       │                 ├─────────────────────────────────►│
       │   HTTP GET      │                                  │
       │◄────────────────┤                                  │
       │ Download file #2│                                  │
       ├────────────────►│                                  │
       │                 │ AgentToServer{PackageStatuses}   │
       │                 ├─────────────────────────────────►│
       │                 │                                  │
       .                 .                                  .

       │   HTTP GET      │                                  │
       │◄────────────────┤                                  │
       │ Download file #N│                                  │
       ├────────────────►│                                  │
       │                 │ AgentToServer{PackageStatuses}   │
       │                 ├─────────────────────────────────►│
       │                 │                                  │
```

The Client MUST always include all packages the Agent has or is processing (downloading or
installing) in PackageStatuses message.

Note that the Client MAY also report the status of packages the Agent has installed
locally, not only the packages it was offered and downloaded from the Server.

#### Calculating Hashes

The Agent and the Server use hashes to identify content of files and packages such
that the Agent can decide what files and packages need to be downloaded.

The calculation of the hashes is performed by the Server. The Server MUST choose
a strong hash calculation method with minimal collision probability (and it may
seed random values into calculation to guarantee hash uniqueness if such
guarantees are needed by the implementation).

The hashes are opaque to the Agent, the Agent never calculates hashes, it only
stores and compares them.

There are 3 levels of hashes:

##### File Hash

The hash of the packages file content. This is stored in the
[content_hash](#downloadablefilecontent_hash) field in
the [DownloadableFile](#downloadablefile-message) message. This value SHOULD be
used by the Agent to determine if the particular file it has is different on the
Server and needs to be re-downloaded.

##### Package Hash

The package hash that identifies the entire package (package name and file content).
This hash is stored in the [hash](#packageavailablehash) field in the
[PackageAvailable](#packageavailable-message) message.

This value SHOULD be used by the Agent to determine if the particular package it
has is different on the Server and needs to be re-downloaded.

##### All Packages Hash

The all packages hash is the aggregate hash of all packages for the particular
Agent. The hash is calculated as an aggregate of all package names and content.
This hash is stored in the [all_packages_hash](#packagesavailableall_packages_hash) field
in the [PackagesAvailable](#packagesavailable-message) message.

This value SHOULD be used by the Agent to determine if any of the packages it has
are different from the ones available on the Server and need to be
re-downloaded.

Note that the aggregate hash does not include the packages that are available on
the Agent locally and were not downloaded from the download Server.

#### Security Considerations

Downloading packages remotely is a potentially dangerous functionality that may be
exploited by malicious actors. If packages contain executable code then a
compromised OpAMP Server may offer a malicious package to the Agent and compel the
Agent to execute arbitrary code.

See Security section for [general recommendations](#general-recommendations) and
recommendations specifically for [code signing](#code-signing) capabilities.

#### PackagesAvailable Message

The message has the following structure:

```
message PackagesAvailable {
    map<string, PackageAvailable> packages = 1;
    bytes all_packages_hash = 2;
}
```

##### PackagesAvailable.packages

A map of packages. Keys are package names.

##### PackagesAvailable.all_packages_hash

Aggregate hash of all remotely installed packages.

The Client SHOULD include this value in subsequent
[PackageStatuses](#packagestatuses-message) messages. This in turn allows the Server
to identify that a different set of packages is available for the Agent and
specify the available packages in the next ServerToAgent message.

This field MUST be always set if the Server supports sending packages to the Agents
and if the Agent indicated it is capable of accepting packages.

#### PackageAvailable Message

This message is an offer from the Server to the Agent to install a new package or
initiate an upgrade or downgrade of a package that the Agent already has. The
message has the following structure:

```protobuf
message PackageAvailable {
    PackageType type = 1;
    string version = 2;
    DownloadableFile file = 3;
    bytes hash = 4;
}
```

##### PackageAvailable.type

The type of the package, either an addon or a top-level package.

```protobuf
enum PackageType {
    TopLevelPackage = 0;
    AddonPackage    = 1;
}
```

##### PackageAvailable.version

The package version that is available on the Server side. The Agent may for
example use this information to avoid downloading a package that was previously
already downloaded and failed to install.

##### PackageAvailable.file

The downloadable file of the package.

##### PackageAvailable.hash

The hash of the package. SHOULD be calculated based on all other fields of the
PackageAvailable message and content of the file of the package. The hash is used by
the Agent to determine if the package it has is different from the package the Server
is offering.

#### DownloadableFile Message

The message has the following structure:

```protobuf
message DownloadableFile {
    string download_url = 1;
    bytes content_hash = 2;
    bytes signature = 3;
    Headers headers = 4; // Status: [Development]
}
```

##### DownloadableFile.download_url

The URL from which the file can be downloaded using HTTP GET request. The Server
at the specified URL SHOULD support range requests to allow for resuming
downloads.

##### DownloadableFile.content_hash

The SHA256 hash of the file content. Can be used by the Agent to verify that the file
was downloaded correctly.

##### DownloadableFile.signature

Optional signature of the file content. Can be used by the Agent to verify the
authenticity of the downloaded file, for example can be the
[detached GPG signature](https://www.gnupg.org/gph/en/manual/x135.html#AEN160).
The exact signing and verification method is Agent specific. See
[Code Signing](#code-signing) for recommendations.

##### DownloadableFile.headers

Status: [Development]

Optional headers to use for the HTTP GET request. Typically used to set access
tokens or other authorization headers. For HTTP-based protocols the Agent
should set these in the request headers.
For example:
key="Authorization", Value="Basic YWxhZGRpbjpvcGVuc2VzYW1l".

### Custom Messages

Status: [Development]

#### Motivation

The OpAMP protocol is intended to cover all aspects of remote management and configuration
of Agents. If more requirements within this core functionality are identified, it is
expected that the protocol will be extended to support this additional behavior in future
releases. However, there are some use cases that require communication with Agents that
will not by supported by the OpAMP protocol because they are not within the scope of Agent
remote management or are highly specific to one agent and not generalizable to other agent
types. CustomCapabilities and CustomMessage allow custom behavior to be implemented
between a Server and Agent without having to open another connection or define an entirely
new protocol.

While interoperability between many Agents and Agent Management Servers is a goal of
OpAMP, additional custom functionality may only be supported by specific Agents and
Servers. Where appropriate, implementers are encouraged to document their use of
CustomMessage so that support can be added to additional platforms. If appropriate, custom
capabilities that are widely supported by Server and Agent implementations may be adopted
as standard OpAMP capabilities in the future.

#### CustomCapabilities

The Agent and Server should both use this message to signal that they support specific
custom capabilities. It is supported on ServerToAgent and AgentToServer. This message
should only be sent when the list of supported custom capabilities changes. When this
message is sent, the list of supported custom capabilities should be updated to match this
list. If this message is never sent, it is assumed that no custom capabilities are
supported.

Capabilities must be identified by a reverse FQDN with optional version information. This
avoids collisions between capabilities introduced by different organizations. The
capability name is composed of the reverse FQDN of the organization that created the
custom capability, a period, and a short name identifying the capability. The version is
optional and is separated from the capability name by a forward slash.

For example, "com.company.capability/v2" identifies version 2 of "capability" created by
"company.com".

If a CustomMessage is received with a capability that is not supported, the message can be
ignored. Support for a capability implies that all corresponding CustomMessages of that
capability are supported. The exact behavior is specific to the custom capability.

```protobuf
message CustomCapabilities {
  repeated string capabilities = 1; // Status: [Development]
}
```

##### CustomCapabilities.capabilities

A list of custom capabilities that are supported. Each capability is a reverse FQDN with
optional version information that uniquely identifies the custom capability and should
match a capability specified in a supported CustomMessage.

#### CustomMessage

CustomMessage allows for custom messages to be sent between the Agent and Server. It is
supported on ServerToAgent and AgentToServer to allow for custom communication in both
directions. It requires that the Agent and Server both agree on the contents and encoding
of the messages. If the capability is not supported or the message type is not recognized,
the message can be ignored.

A CustomMessage identifies the type of message and contains binary data with the contents
of the message. The format of the data will depend on the message type and is outside of
the scope of the OpAMP protocol.

```protobuf
message CustomMessage {
    string capability = 1;
    string type = 2;
    bytes data = 3;
}
```

##### CustomMessage.capability

A reverse FQDN that uniquely identifies the capability and matches one of the capabilities
in the CustomCapabilities message.

##### CustomMessage.type

Type of message within the capability. The capability defines the types of custom messages
that are used to implement the capability. The type must only be unique within the
capability.

##### CustomMessage.data

Binary data of the message. The capability must specify the format of the contents of the
data for each custom message type it defines.

#### Examples

The following examples describe possible uses of CustomCapabilities and CustomMessage but
are not intended to be part of any specification. In these examples, "io.opentelemetry."
is used to create the FQDN of the message types but each vendor is expected to use their
own FQDN.

##### Pause/Resume Example

Suppose an Agent supports the ability to pause and resume telemetry collection. When
paused, no telemetry data is collected or sent. To allow a Server to control this behavior, a CustomCapability using CustomMessages could be introduced.

###### Agent Connection

On connection, the Agent sends an AgentToServer message with CustomCapabilities including
the capabilities "io.opentelemetry.pause". In response, the Server sends a ServerToAgent
message with a CustomCapabilities message including "io.opentelemetry.pause".

```
CustomCapabilities {
  capabilities: ["io.opentelemetry.pause"]
}
```

###### Pause

Server sends a ServerToAgent message containing the following CustomMessage with type
"pause". No data is sent because this is a simple command with no additional information.

```
CustomMessage {
  capability: "io.opentelemetry.pause"
  type: "pause"
}
```

If the Agent supports this message and is able to successfully pause, it returns an
AgentToServer message containing a CustomMessage with type "status" and with data
containing a binary JSON-encoded response with the new paused state.

```
CustomMessage {
  capability: "io.opentelemetry.pause"
  type: "status"
  data: {
    "paused": true
  }
}
```

If the Agent supports this message but encounters an error trying to pause, it returns an
AgentToServer message containing a CustomMessage with type "error" and with data
containing a binary JSON-encoded response containing an error message.

```
CustomMessage {
  capability: "io.opentelemetry.pause"
  type: "error"
  data: {
    "error": "Unable to pause"
  }
}
```

###### Resume

Similar to Pause but the ServerToAgent message contains a CustomMessage with type
"resume" and on success the new status would be `{ "paused": false }`.

```
CustomMessage {
  capability: "io.opentelemetry.pause"
  type: "resume"
}
```

##### Service Discovery Example

Service discovery involves discovering running services that are accessible to the Agent
for which the Agent could collect telemetry. In this example the Server will send a
message to the Agent to ask for available services. It will expect a response from the
Agent containing information about the available services.

###### Agent Connection

On connection, the Agent sends a CustomCapabilities message including the capability
"io.opentelemetry.discovery". In response, the Server sends a CustomCapabilities message
including that also includes the capability "io.opentelemetry.discovery". Since this
capability is supported by both the Agent and Server, messages associated with this
capability can be sent to support discovery.

###### FindServices

Server sends a ServerToAgent message containing the following CustomMessage. No data is
sent because this is a simple request with no additional information.

```
CustomMessage {
  capability: "io.opentelemetry.discovery"
  type: "find_services"
}
```

###### FindServicesResponse

After discovering services, the Agent sends an AgentToServer message containing a
CustomMessage with the binary data containing the JSON encoding of a complex data
structure containing information about all of the services it discovered. This data
structure would also include an optional field for errors encountered during discovery.

```
CustomMessage {
  capability: "io.opentelemetry.discovery"
  type: "available_services"
  data: { ... }
}
```

### AvailableComponents Message

Status: [Development]

Each Agent may be composed of multiple sub-components. These components may have
their own associated metadata. The AvailableComponents message allows the Agent
to inform the OpAMP server about which components the Agent contains.

The AvailableComponents message has the following structure:

```protobuf
message AvailableComponents {
    map<string, ComponentDetails> components = 1;
    bytes hash = 2;
 }
```

#### AvailableComponents.components

The components field contains a map of a unique ID
to a [ComponentsDetails](#componentdetails-message) message.
This field may be omitted from the message if the OpAMP server has not explicitly
requested it by setting the ReportAvailableComponents flag in the preceding
ServerToAgent message.

##### Examples

###### OpenTelemetry Collector

Here is an example of how ComponentDetails could hold information regarding the included
components for a custom build of the [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector):

```jsonc
{
  "receivers": {
    "sub_component_map": {
      "hostmetrics": {
        "metadata": {
          "code.namespace": "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver@v0.107.0",
        }
      }
    }
  },
  "processors": {
    "sub_component_map": {
      "batch": {
        "metadata": {
          "code.namespace": "go.opentelemetry.io/collector/processor/batchprocessor@v0.107.0"
        }
      },
      "transform": {
        "metadata": {
          "code.namespace": "github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor@v0.107.0"
        }
      },
    }
  },
  "exporters": {
    "sub_component_map": {
      "nop": {
        "metadata": {
          "code.namespace": "go.opentelemetry.io/collector/exporter/nopexporter@v0.107.0"
        }
      }
    }
  }
  // ... Component list continues for extensions and collectors ...
}
```

###### Fluent Bit

Here's an example of how Fluent Bit could report what components it has available.

```jsonc
{
  "input": {
    "sub_component_map": {
      "tail": {}
    }
  },
  "parser": {
    "sub_component_map": {
      "json": {}
    }
  },
  "filter": {
    "sub_component_map": {
      "lua": {},
      "modify": {}
    }
  },
  "output": {
    "sub_component_map": {
      "null": {},
      "file": {},
    }
  }
}
```

#### AvailableComponents.hash

The agent-calculated hash of the components field. The agent MUST include this hash
if it has the ability to report the components it has available.
Its format is determined by the agent.
The hash SHOULD be the same for any agent that has the same set of components.

#### Initial Handshake

In order to reduce the amount of data transmitted, the AvailableComponents message
does not initially contain the entire components map. Instead, the AvailableComponents
message will have the agent computed hash set, with an empty map for components.

The initial AgentToServer message SHOULD contain the AvailableComponents message
with just the agent computed hash set.

The OpAMP server may use this hash to determine whether it remembers the set of
AvailableComponents or not. If the hash is not found on the OpAMP server, the server
may request for the full components map to be reported by setting the ReportAvailableComponents
flag in the ServerToAgent message. If this flag is specified, then the Agent will
populate the components field with a full description of the available components.

The server may then optionally persist the components for this hash, so that the
server does not need to request them again on subsequent connects.

The AvailableComponents message is also subject to [status compression](#agent-status-compression),
and may be omitted if the hash has not changed from its previous value.

#### ComponentDetails Message

The ComponentDetails messaged describes a component of the Agent.

The structure of ComponentDetails DOES NOT need to be a 1-to-1 match with
the ComponentHealth structure. ComponentHealth generally refers to currently running instances of
components, while ComponentDetails refers to the available types of components,
which may not be necessarily running currently. It is also possible that the same component
type may have more than one running instance.

The ComponentDetails message has the following structure:

```protobuf
message ComponentDetails {
    repeated KeyValue metadata = 1;
    map<string, ComponentDetails> sub_component_map = 2;
}
```

##### ComponentDetails.metadata

Extra key/value pairs that may be used to describe the component.

The key/value pairs are SHOULD conform to semantic conventions where applicable, see
the [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/) for more information.

##### ComponentDetails.sub_component_map

A map of unique component ID to sub component details. It can nest as deeply as needed to
describe the underlying system.

## Connection Management

### Establishing Connection

The Client connects to the Server by establishing an HTTP(S) connection.

If WebSocket transport is used then the connection is upgraded to WebSocket as
defined by WebSocket standard.

After the connection is established the Client MUST send the first
[status report](#status-reporting) and expect a response to it.

If the Client is unable to establish a connection to the Server it SHOULD retry
connection attempts and use exponential backoff strategy with jitter to avoid
overwhelming the Server.

When retrying connection attempts the Client SHOULD honour any
[throttling](#throttling) responses it receives from the Server.

### Closing Connection

#### WebSocket Transport, OpAMP Client Initiated

To close a connection the Client MUST first send an AgentToServer message with
agent_disconnect field set. The Client MUST then send a WebSocket
[Close](https://datatracker.ietf.org/doc/html/rfc6455#section-5.5.1) control
frame and follow the procedure defined by WebSocket standard.

#### WebSocket Transport, Server Initiated

To close a connection the Server MUST then send a WebSocket
[Close](https://datatracker.ietf.org/doc/html/rfc6455#section-5.5.1) control
frame and follow the procedure defined by WebSocket standard.

#### Plain HTTP Transport

The Client is considered logically disconnected as soon as the OpAMP HTTP
response is completed. It is not necessary for the Client to send AgentToServer
message with agent_disconnect field set since it is always implied anyway that
the Client connection is gone after the HTTP response is completed.

HTTP keep-alive may be used by the Client and the Server but it has no effect on
the logical operation of the OpAMP protocol.

The Server may use its own business logic to decide what it considers an active
Agent (e.g. an Client that continuously polls) vs an inactive Agent (e.g. a
Client that has not made an HTTP request for a specific period of time). This business
logic is outside the scope of OpAMP specification.

### Restoring WebSocket Connection

If an established WebSocket connection is broken (disconnected) unexpectedly the
Client SHOULD immediately try to re-connect. If the re-connection fails the Client
SHOULD continue connection attempts with backoff as described in
[Establishing Connection](#establishing-connection).

### Duplicate WebSocket Connections

Each Client instance SHOULD connect no more than once to the Server. If the Client
needs to re-connect to the Server the Client MUST ensure that it sends an
AgentDisconnect message first, then closes the existing connection and only then
attempts to connect again.

The Server MAY disconnect or deny serving requests if it detects that the same
Client instance has more than one simultaneous connection or if multiple Agent
instances are using the same instance_uid.

The Server SHOULD detect duplicate `instance_uid`s (which may happen for example
when Agents are using bad UID generators or due to cloning of the VMs where the
Agent runs). When a duplicate `instance_uid` is detected, Server SHOULD generate
a new `instance_uid`, and send it as `new_instance_uid` value of AgentIdentification.

### Authentication

Status: [Beta]

The Client and the Server MAY use authentication methods supported by HTTP, such
as [Basic](https://datatracker.ietf.org/doc/html/rfc7617) authentication or
[Bearer](https://datatracker.ietf.org/doc/html/rfc6750) authentication. The
authentication happens when the HTTP connection is established before it is
upgraded to a WebSocket connection.

The Server MUST respond with
[401 Unauthorized](https://datatracker.ietf.org/doc/html/rfc7235#section-3.1) if
the Client authentication fails.

### Bad Request

If the Server receives a malformed AgentToServer message the Server SHOULD
respond with a ServerToAgent message with [error_response](#servertoagenterror_response)
set accordingly. The [type](#servererrorresponsetype) field MUST be set to BAD_REQUEST and
[error_message](#servererrorresponseerror_message) SHOULD be a human readable description
of the problem with the AgentToServer message.

The Client SHOULD NOT retry sending an AgentToServer message to which it received
a BAD_REQUEST response.

### Retrying Messages

The Client MAY retry sending AgentToServer message if:

* AgentToServer message that requires a response was sent, however no response
  was received within a reasonable time (the timeout MAY be configurable).
* AgentToServer message that requires a response was sent, however the
  connection was lost before the response was received.
* After receiving an UNAVAILABLE response from the Server as described in the
  [Throttling](#throttling) section.

For messages that require a response if the Server receives the same message
more than once the Server MUST respond to each message, not just the first
message, even if the Server detects the duplicates and processes the message
once.

Note that the Client is not required to keep a growing queue of messages that it
wants to send to the Server if the connection is unavailable. The Client
typically only needs to keep one up-to-date message of each kind that it wants
to send to the Server and send it as soon as the connection is available.

For example, the Client should keep track of the Agent's status and compose a
AgentToServer message that is ready to be sent at the first opportunity. If the
Client is unable to send the AgentToServer message (for example if the connection
is not yet available) the Client does not need to create a new AgentToServer every
time the Agent's status changes and keep all these AgentToServer messages in a
queue ready to be sent. The Client simply needs to keep one up-to-date
AgentToServer message and send it at the first opportunity. This of course
requires the AgentToServer message to contain all changes since it was last
reported and to correctly reflect the current (last) state of the Agent.

Similarly, all other Agent reporting capabilities, such as Addon Status
Reporting or Agent Package Installation Status Reporting require the Client to
only keep one up-to-date status message and send it at the earliest opportunity.

The exact same logic is true in the opposite direction: the Server normally only
needs to keep one up-to-date message of a particular kind that it wants to
deliver to the Agent and send it as soon as the connection to the Client is
available.

### Throttling

#### WebSocket Transport

When the Server is overloaded and is unstable to process the AgentToServer
message it SHOULD respond with an ServerToAgent message, where
[error_response](#servertoagenterror_response) is filled with
[type](#servererrorresponsetype) field set to UNAVAILABLE.
The Client SHOULD disconnect, wait, then reconnect again and resume its
operation. The retry_info field may be optionally set with
retry_after_nanoseconds field specifying how long the Client SHOULD wait before
reconnecting:

```protobuf
message RetryInfo {
    uint64 retry_after_nanoseconds = 1;
}
```

If retry_info is not set then the Client SHOULD implement an exponential backoff
strategy to gradually increase the interval between retries.

#### Plain HTTP Transport

In the case when plain HTTP transport is used as well as when WebSocket is used
and the Server is overloaded and is unable to upgrade the HTTP connection to
WebSocket the Server MAY return
[HTTP 503 Service Unavailable](https://datatracker.ietf.org/doc/html/rfc7231#page-63)
or
[HTTP 429 Too Many Requests](https://datatracker.ietf.org/doc/html/rfc6585#section-4)
response and MAY optionally set
[Retry-After](https://datatracker.ietf.org/doc/html/rfc7231#section-7.1.3)
header to indicate when SHOULD the Client attempt to reconnect. The Client SHOULD
honour the corresponding requirements of HTTP specification.

Note: a Retry-After header SHOULD be used only for the client's attempts to reconnect to the server.
A client should not attempt to send regular [heartbeat](#opampconnectionsettingsheartbeat_interval_seconds) messages while the Agent is reconnecting.

The minimum recommended retry interval is 30 seconds.

## Security

Remote configuration, downloadable packages are a significant
security risk. By sending a malicious Server-side configuration or a malicious
package the Server may compel the Agent to perform undesirable work. This section
defines recommendations that reduce the security risks for the Agent.

Guidelines in this section are optional for implementation, but are highly
recommended for sensitive applications.

### General Recommendations

We recommend that the Agent employs the zero-trust security model and does not
automatically trust the remote configuration or other offers it receives from
the Server. The data received from the Server should be verified and sanitized
by the Agent in order to limit and prevent the damage that may be caused by
malicious actors. We recommend the following:

* The Agent should run at the minimum possible privilege to prevent itself from
  accessing sensitive files or perform high privilege operations. The Agent
  should not run as root user, otherwise a compromised Agent may result in total
  control of the machine by malicious actors.
* If the Agent is capable of collecting local data it should limit the
  collection to a specific set of directories. This limitation should be locally
  specified and should not be overridable via remote configuration. If this rule
  is not followed the remote configuration functionality may be exploited to
  access sensitive information on the Agent's machine.
* If the Agent is capable of executing external code located on the machine
  where it runs and this functionality can be specified in the Agent's
  configuration then the Agent should limit such functionality only to specific
  scripts located in a limited set of directories. This limitation should be
  locally specified and should not be overridable via remote configuration. If
  this rule is not followed the remote configuration functionality may be
  exploited to perform arbitrary code execution on the Agent's machine.

### Configuration Restrictions

The Agent is recommended to restrict what it may be compelled to do via remote
configuration.

Particularly, if it is possible via a configuration to ask the Agent to collect
data from the machine it runs on (as it is often the case for telemetry
collecting Agents) then we recommend to have Agent-side restrictions as to what
directories or files the Agent is allowed to collect. Upon receiving a remote
config the Agent must validate the configuration against the list of
restrictions and refuse to apply the configuration either fully or partially if
it violates the restrictions or sanitize the configuration such that it does not
collect data from prohibited directories or files.

Similarly, if the configuration provides means to order the Agent to execute
processes or scripts on the machine it runs on we recommend to have Agent-side
restrictions as to what executable files from what directories the Agent is
allowed to run.

It is recommended that the restrictions are specified in the form of "allow
list" instead of the "deny list". The restrictions may be hard-coded or may be
end-user definable in a local config file. It should not be possible to override
these restrictions by sending a remote config from the Server to the Agent.

### Opt-in Remote Configuration

It is recommended that remote configuration capabilities are not enabled in the
Agent by default. The capabilities should be opt-in by the user.

### Code Signing

Any executable code that is part of a package should be signed
to prevent a compromised Server from delivering malicious code to the Agent. We
recommend the following:

* Any downloadable executable code (e.g. executable packages)
  need to be code-signed. The actual code-signing and verification mechanism is
  Agent specific and is outside the concerns of the OpAMP specification.
* The Agent should verify executable code in downloaded files to ensure the code
  signature is valid.
* The downloadable code can be signed with the signature included in the file content or
  have a detached signature recorded in the DownloadableFile
  message's [signature](#downloadablefilesignature) field. Detached signatures may be used
  for example with [GPG signing](https://www.gnupg.org/gph/en/manual/x135.html#AEN160).
* If Certificate Authority is used for code signing it is recommended that the
  Certificate Authority and its private key is not co-located with the OpAMP
  Server, so that a compromised Server cannot sign malicious code.
* The Agent should run any downloaded executable code (the packages and or any
  code that it runs as external processes) at the minimum possible privilege to
  prevent the code from accessing sensitive files or perform high privilege
  operations. The Agent should not run downloaded code as root user.

## Interoperability

### Interoperability of Partial Implementations

OpAMP defines a number of capabilities for the Agent and the Server. Most of
these capabilities are optional. The Agent or the Server should be prepared that
the peer does not support a particular capability.

Both the Agent and the Server indicate the capabilities that they support during
the initial message exchange. The Client sets the capabilities bit-field in the
AgentToServer message, the Server sets the capabilities bit-field in the
ServerToAgent message.

Each set bit in the capabilities field indicates that the particular capability
is supported. The list of Agent capabilities is [here](#agenttoservercapabilities).
The list of Server capabilities is [here](#servertoagentcapabilities).

After the Server learns about the capabilities of the particular Agent the
Server MUST stop using the capabilities that the Agent does not support.

Similarly, after the Agent learns about the capabilities of the Server the Agent
MUST stop using the capabilities that the Server does not support.

The specifics of what in the behavior of the Agent and the Server needs to
change when they detect that the peer does not support a particular capability
are described in this document where relevant.

### Interoperability of Future Capabilities

There are 2 ways OpAMP enables interoperability between an implementation of the
current version of this specification and an implementation of a future,
extended version of OpAMP that adds more capabilities that are not described in
this specification.

#### Ignorable Capability Extensions

For the new capabilities that extend the functionality in such a manner that
they can be silently ignored by the peer a new field may be added to any
Protobuf message. The sender that implements this new capability will set the
new field. A recipient that implements an older version of the specification
that is unaware of the new capability will simply ignore the new field. The
Protobuf encoding ensures that the rest of the fields are still successfully
deserialized by the recipient.

#### Non-Ignorable Capability Extensions

For the new capabilities that extend the functionality in such a manner that
they cannot be silently ignored by the peer a different approach is used.

The capabilities fields in AgentToServer and ServerToAgent messages contains a
number of reserved bits. These bits SHOULD be used for indicating support of new
capabilities that will be added to OpAMP in the future.

The Client and the Server MUST set these reserved bits to 0 when sending the
message. This allows the recipient, which implements a newer version of OpAMP to
learn that the sender does not support the new capability and adjust its
behavior correspondingly.

The AgentToServer and ServerToAgent messages are the first messages exchanged by
the Client and Server which allows them to learn about the capabilities of the
peer and adjust their behavior appropriately. How exactly the behavior is
adjusted for future capabilities MUST be defined in the future specification of
the new capabilities.

#### Protobuf Schema Stability

The specification provides the follow stability guarantees of the
[Protobuf definitions](proto/opamp.proto) for OpAMP 1.0:

- Field types, numbers and names will not change.
- Names of messages and enums will not change.
- Numbers assigned to enum choices will not change.
- Names of enum choices will not change.
- The location of messages and enums, i.e. whether they are declared at the top lexical
  scope or nested inside another message will not change.
- Package names and directory structure will not change.
- `optional` and `repeated` declarators of existing fields will not change.
- No existing symbol will be deleted.

Note that the above guarantees do not apply to messages and fields which are
labeled `[Beta]`. [Beta] message and fields are subject to weaker guarantees as defined
in the [maturity matrix][beta].

Future versions of the OpAMP specification may be extended by modifying the
Protobuf schema defined in this specification version. The following Protobuf schema
changes are allowed, provided that they comply with the interoperability requirements
defined elsewhere in this specification:

- Adding new fields to existing messages.
- Adding new messages or enums.
- Adding new choices to existing enums.
- Adding new choices to existing oneof fields.

## Future Possibilities

Define specification for Concentrating Proxy that can serve as intermediary to
reduce the number of connections to the Server when a very large number
(millions and more) of Agents are managed.

## References

### Agent Management

* Splunk
  [Deployment Server](https://docs.splunk.com/Documentation/Splunk/latest/Updating/Aboutdeploymentserver)
* Centralized Configuration of vRealize
  [Log Insight Agents](https://docs.vmware.com/en/vRealize-Log-Insight/8.10/com.vmware.log-insight.agent.admin.doc/GUID-40C13E10-1554-4F1B-B832-69CEBF85E7A0.html)
* Google Cloud
  [Guest Agent](https://github.com/GoogleCloudPlatform/guest-agent) uses HTTP
  [long polling](https://cloud.google.com/compute/docs/metadata/querying-metadata#waitforchange)

### Configuration Management

* [Uber Flipr](https://www.uber.com/blog/flipr/)
* Facebook's
  [Holistic Configuration Management](https://research.facebook.com/file/877841159827226/holistic-configuration-management-at-facebook.pdf)
  (push)

### Security and Certificate Management

* [mTLS in Go](https://web.archive.org/web/20240223142649/https://kofo.dev/how-to-mtls-in-golang)
* [ACME certificate management protocol](https://datatracker.ietf.org/doc/html/rfc8555)
* [ACME for client certificates](https://datatracker.ietf.org/doc/draft-moriarty-acme-client/)

### Cloud Provider Support

* [AWS](https://aws.amazon.com/elasticloadbalancing/features/)
* [GCP](https://cloud.google.com/appengine/docs/flexible/go/using-websockets-and-session-affinity)
* [Azure](https://docs.microsoft.com/en-us/azure/application-gateway/application-gateway-websocket)

### Other

* [Websocket Load Balancing](https://pdf.sciencedirectassets.com/280203/1-s2.0-S1877050919X0006X/1-s2.0-S1877050919303576/main.pdf?X-Amz-Security-Token=IQoJb3JpZ2luX2VjEI3%2F%2F%2F%2F%2F%2F%2F%2F%2F%2FwEaCXVzLWVhc3QtMSJHMEUCIAhC7%2Bztk8aH29lDsWYFIHLt97kwOE4PoWkiPfH2OTQwAiEA65oLMq1RhzF6b5pSixhnPVLT9G2iKkG145XtdpW4d4IqgwQIpv%2F%2F%2F%2F%2F%2F%2F%2F%2F%2FARAEGgwwNTkwMDM1NDY4NjUiDDtEVrp4vXmh0hvwWyrXAxnfLN4%2BsMMF7wxoXOiBFQjn%2FJLpSLUIWghc87%2Bx2tbvdCIC%2BQV4JCY9rOK3p9rogqh9yoI2yem4SHASzL%2BQUQMOiGWagk%2FzyCNdS0y%2FLzHkKDahvRMJGKxWeXErbsuvPCufnbDpNHmKD0vnT5sqpOoM64%2FJVxvd9QYx48xasNMtXZ8%2BFm9wPpNQnsWSEZKYiOKLaLfnATzcXADJmOCTVQbwZoT4%2BFKWcoujBxSBHE9kw7S749ywQ9bOtgNWid5R2dj0z%2Br6C63SnBS3IdMSZ2qO4H3XTYY5pbfNCfR57zKIdwyp3zLJr5%2BtTEz1YR9FXwWF9niDEr0v2qu%2FlL7%2BGHsak8UQ4hZ0BFlZtcIRNW1lpZd9bNSINb3d6MnGeYrkhxQVP0KcZsowP9672IYzuMD4nK1X4Hv7bMqeO7ojuSf%2F2ND9NXn0Ldr%2BX0lzESv10LyhElCGfFJ4EZjIxYOKZdee1Zc1USdj1kNx1OC0cefIN1ixiA0OIbtWVz1lI6n1LYpngeUYngGP0ZFb%2Br%2FbleC3WarDHWIn4NNjI1aQW3P9fTmKEan3b3skRIBbwM8%2FrwRJGYQ03JaCKuU4xbogz9uEL%2BbpJ1SB7En8pS8xuSiE1kzvnsF0FTCEvMSIBjqlAadtZOgWRUk2FxdoYsCK43DYqD6zjbDrRBfyIXTJGlJYKt5iR3SCi8ySacO1aPZhah9ir179nYi5dVYnf5c6%2Fe8Q5Mo1uRtisouWJZSjAOhmRY7a76fSqyHwj088aI5t1pcempNCOnsM4SfyrZJ9UE%2FKfb5YsJ71VwRPZ%2BXZ%2FvZnQlW7e6NJqWswhre0pQftkShN%2BbpE%2FTzusekzm6q3w6b3ynUN8A%3D%3D&X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Date=20210809T134614Z&X-Amz-SignedHeaders=host&X-Amz-Expires=299&X-Amz-Credential=ASIAQ3PHCVTY2T5F5OYZ%2F20210809%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Signature=6098b604ebac38723d26ae66e527b397312a6371ad19e1a4fbfe94ca9c61e1a9&hash=ebd5b943d3aff77c6bfb8853fab1598db53996f5f018d688364a41dd71c15d92&host=68042c943591013ac2b2430a89b270f6af2c76d8dfd086a07176afe7c76c2c61&pii=S1877050919303576&tid=spdf-3c0a3a1a-bd3b-40d0-af0d-48a46859c89a&sid=d21b79c59bbb0348b79945c084cc3b66983agxrqa&type=client)

[beta]: https://opentelemetry.io/docs/specs/otel/document-status/

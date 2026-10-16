# ADR: Integration of Linux System Logs into LaaS via Fluent Bit

**Status:** Proposed – Option A Selected, Pending Approval

**Scope:** EC Linux System Logs – SHG-GDS / BJV-VIA

**Selected Solution:** Deploy Fluent Bit on existing Rsyslog servers using Tail Input

## 1. Context & Problem Statement

The Enterprise Computing (EC) Linux team currently operates centralized Rsyslog servers in SHG-GDS and BJV-VIA to collect Linux system logs.

Linux servers forward their system logs to the existing Rsyslog infrastructure, where logs are stored locally with a retention period of 30 days and forwarded to Splunk for SIEM purposes.

However, the current architecture presents several challenges:

- **Splunk Decommissioning:** Splunk is planned to be decommissioned, requiring an alternative centralized logging capability for Linux operational logs.
- **Limited Operational Visibility:** Linux SREs currently lack a dedicated, centralized platform to efficiently search, investigate, and troubleshoot system logs across Linux servers.
- **Regulatory Compliance:** Centralizing Linux system logs within the approved Logging as a Service (LaaS) platform aligns with the firm's regulatory and internal control requirements for log management, access control, retention, and traceability.
- **Multi-Tenancy:** The existing Rsyslog infrastructure does not separate Linux logs by Legal Entity. LaaS requires logs to be routed to the appropriate Loki tenants.

The objective is to integrate Linux system logs into the existing LaaS platform, powered by Fluent Bit and Grafana Loki, while minimizing changes to the current Linux logging infrastructure.

The Linux team will provide a dedicated **SystemID authorized to write to multiple designated Linux-related Loki tenants**. Fluent Bit will use this identity for authentication and route logs based on their validated Legal Entity and tenant mapping.

The integration should preserve the existing Rsyslog collection process during the transition, support the planned Splunk decommissioning, and provide centralized log search and troubleshooting capabilities through Grafana/LaaS.

**Scope and Boundaries**

In scope:
- Linux system logs currently collected by the EC Rsyslog infrastructure.
- Fluent Bit collection, parsing, enrichment, and tenant-based routing.
- Integration with existing LaaS/Loki tenants.
- Operational resilience, monitoring, and security controls.

Out of scope:
- Non-Linux infrastructure log sources.
- Redesign of existing Linux-to-Rsyslog forwarding.
- Changes to the Splunk decommissioning schedule or SIEM replacement strategy.
- Changes to existing Loki tenant boundaries or authorization policies.

## 2. Decision Drivers

The following factors drive the architectural decision:

**1. Business and Regulatory Alignment**
- Provide a centralized logging capability for Linux SRE operations.
- Support the planned Splunk decommissioning.
- Meet applicable internal control and regulatory requirements for centralized log management, subject to confirmation of retention, access control, and auditability requirements.

**2. Minimal Infrastructure Changes**
- Reuse the existing Rsyslog infrastructure wherever possible.
- Avoid additional servers, client-side configuration changes, and unnecessary firewall modifications.
- Preserve existing Linux log collection during the transition.

**3. Availability and Data Reliability**
- Minimize additional log loss risks.
- Support recovery from Fluent Bit restarts and temporary Loki unavailability.
- Avoid impacting the existing Rsyslog service if Fluent Bit becomes unavailable.

**4. Scalability and Performance**
- Support the current Linux log volume and future growth.
- Manage CPU, memory, file descriptors, and disk I/O on shared Rsyslog servers.
- Ensure that Fluent Bit does not materially degrade Rsyslog performance.

**5. Security and Multi-Tenancy**
- Route Linux logs to the correct Loki tenant based on validated source information.
- Use the dedicated SystemID provided by the Linux team to authenticate against authorized Linux-related tenants.
- Prevent unauthorized or incorrect cross-tenant log ingestion.

**6. Operational Supportability and Cost**
- Simplify deployment, monitoring, troubleshooting, and maintenance.
- Minimize infrastructure and implementation costs.
- Provide a straightforward rollback mechanism.

## 3. Considered Options

Three integration options were evaluated.

### Option A – Deploy Fluent Bit on Existing Rsyslog Servers Using Tail Input

Fluent Bit is deployed directly on the existing EC Rsyslog servers.

Rsyslog continues receiving Linux logs through its existing listeners and writing them to local files.

Fluent Bit uses the Tail input plugin to collect these files, extract source information, identify the corresponding Legal Entity, and forward records to the appropriate Loki tenant.

The dedicated Linux SystemID is used for authentication across the approved Linux-related tenants.

**Architecture:** See Mermaid Diagram – Option A.

**Data Flow:**

Linux Servers → Existing Rsyslog → Local Linux Log Files → Fluent Bit (Tail / Parsing / Tenant Routing) → LaaS / Loki

The existing Rsyslog-to-Splunk forwarding path remains operational until the separate Splunk decommissioning process is completed.

This option requires no client-side configuration changes and avoids introducing additional inbound syslog listeners.

Potential concerns include file descriptor usage, Tail performance, file rotation, and additional resource consumption on the existing Rsyslog servers.

### Option B – Deploy Fluent Bit on Existing Rsyslog Servers Using TCP/UDP/Syslog Input

Fluent Bit is deployed on the existing EC Rsyslog servers but receives Linux logs through network input plugins rather than monitoring local files.

As Rsyslog already owns the existing listener ports, Fluent Bit cannot independently use the same IP/port/protocol combination under a conventional listener configuration.

The feasible implementation would require Rsyslog to forward an additional copy of Linux logs to Fluent Bit through a separate local TCP/Syslog listener.

**Architecture:** See Mermaid Diagram – Option B.

**Data Flow:**

Linux Servers → Existing Rsyslog → Local Forwarding → Fluent Bit (TCP/Syslog Input / Parsing / Tenant Routing) → LaaS / Loki

This option avoids Tail-related file monitoring overhead and reuses the existing servers and client connectivity.

However, it requires additional Rsyslog forwarding configuration, queue management, and failure-handling logic.

Tenant identification and routing are still required.

### Option C – Deploy Dedicated Fluent Bit Servers

One or two dedicated servers are provisioned to host Fluent Bit independently of the existing Rsyslog infrastructure.

Linux clients are configured to forward logs to both the existing Rsyslog servers and the new Fluent Bit endpoints.

Fluent Bit receives logs through TCP/UDP/Syslog input plugins, performs parsing and tenant routing, and forwards them to LaaS.

Dedicated listener ports may be used to assist Legal Entity separation, subject to validated source mapping.

**Architecture:** See Mermaid Diagram – Option C.

**Data Flow:**

Existing: Linux Servers → Rsyslog → Splunk

Additional: Linux Servers → Dedicated Fluent Bit Servers → LaaS / Loki

This option provides infrastructure separation and greater flexibility for future scaling.

However, it requires additional servers, client-side Rsyslog configuration changes, network connectivity, and potentially F5 load balancing to support high availability.

## 4. Decision Outcome

**Chosen Option: Option A – Fluent Bit on Existing Rsyslog Servers Using Tail Input.**

Following technical evaluation and initial testing, Option A has been selected as the preferred solution for integrating Linux system logs into LaaS.

The primary justifications are:

**1. Reuse of Existing Infrastructure**

Fluent Bit can consume the Linux log files already generated by Rsyslog without modifying existing Linux client configurations or syslog listener ports.

This minimizes implementation complexity and dependencies on other infrastructure teams.

**2. Minimal Disruption to Existing Services**

Fluent Bit operates as an additional log consumer, independently of the current Rsyslog-to-Splunk forwarding path.

The existing logging service can continue operating during implementation and the Splunk decommissioning transition.

**3. Reliable Recovery from Persisted Logs**

Fluent Bit Tail input supports persistent offset tracking, allowing collection to resume after restarts.

Filesystem buffering and retry mechanisms can provide additional resilience during temporary Loki outages.

These mechanisms improve downstream recovery but do not eliminate log loss risks that occur before Rsyslog successfully persists incoming messages.

**4. Secure Multi-Tenant Integration**

The Linux team will provide a dedicated SystemID with write access to the designated Linux-related Loki tenants.

Fluent Bit will determine the appropriate target tenant using validated hostname and Legal Entity mapping rules.

Authentication will use the approved SystemID, while each log record must be routed under its correct Loki tenant identity.

The SystemID's multi-tenant permissions do not replace per-record tenant validation.

**5. Lower Cost and Operational Complexity**

Option A avoids additional log collection servers, client-side changes, and high availability infrastructure dedicated to Fluent Bit.

**6. Simplified Rollback**

Fluent Bit can be disabled or removed without changes to the existing Rsyslog listener configuration.

### Implementation Controls

The production implementation will include:

- Tail input configuration targeting approved Linux log directories only.
- Persistent offset tracking and log rotation handling.
- Filesystem buffering and retry configuration.
- Validated hostname-to-Legal-Entity-to-Loki-tenant mapping.
- Authentication using the dedicated Linux SystemID.
- Fail-closed handling for unknown or unauthorized tenant mappings.
- Monitoring of ingestion failures, backlog, CPU, memory, file descriptors, and disk utilization.
- Recovery, performance, and coexistence testing before production rollout.

## 5. Consequences

### Positive Consequences

**Centralized Linux Log Visibility**

Linux SREs gain centralized access to system logs through Grafana/LaaS, improving troubleshooting, investigation, and operational efficiency.

**Support for Splunk Decommissioning**

LaaS provides the centralized operational log collection and search capability required for the Linux logging transition away from Splunk.

This does not imply replacement of Splunk's security analytics or other SIEM functions.

**Regulatory and Internal Control Alignment**

The solution uses the approved LaaS platform for centralized storage, access control, and log retrieval.

Applicable retention periods, auditability, and data access requirements must be validated as part of implementation.

**Infrastructure Reuse**

No dedicated Fluent Bit servers or client-side changes are required.

**Operational Independence**

Fluent Bit can be managed independently while the existing Rsyslog service continues collecting Linux system logs.

### Negative Consequences and Risks

**Additional Resource Consumption**

Fluent Bit introduces additional CPU, memory, file descriptor, and disk I/O usage on existing Rsyslog servers.

**Tail Input and Log Rotation Dependencies**

Incorrect offset tracking, file rotation handling, or insufficient buffering capacity may result in missing or duplicated log records.

**Shared Infrastructure Dependencies**

Both the existing Rsyslog collection process and the new LaaS integration rely on the same underlying servers.

The integration does not eliminate existing UDP-related log loss risks.

**Tenant Mapping Requirements**

Since the existing Rsyslog architecture does not separate Linux logs by Legal Entity, explicit source identification and routing rules are required.

**Increased Dependency on LaaS**

Following Splunk decommissioning, LaaS becomes the primary centralized operational log search platform for Linux SREs.

Its availability, retention, and recovery capabilities therefore become increasingly important.

### Actions and Dependencies

| Action | Responsible Team |
|---|---|
| Confirm Linux log paths, rotation, and local retention | Linux |
| Provide dedicated SystemID with authorized Linux tenant access | Linux / LaaS |
| Confirm hostname-to-Legal-Entity-to-tenant mapping | Linux / Observability |
| Deploy and configure Fluent Bit | Observability |
| Validate Tail performance and resource consumption | Observability / Linux |
| Validate Loki connectivity, authentication, and tenant isolation | Observability / LaaS |
| Validate restart, backlog, and outage recovery | Observability / Linux |
| Confirm regulatory retention and access control requirements | Relevant control owners / Observability |
| Coordinate Splunk transition and decommissioning | Relevant platform owners |

### Reversibility

The Fluent Bit integration itself is highly reversible: it can be disabled without modifying existing Rsyslog listeners or client forwarding configurations.

However, after Splunk is decommissioned, removing Fluent Bit would also remove the new centralized Linux operational logging path. Any subsequent rollback must therefore preserve the required log access and retention capabilities.

## 6. Pros and Cons of the Options

### Option A – Fluent Bit Tail Input (Selected)

**Pros**
- Reuses existing Rsyslog servers and log files.
- No Linux client configuration changes.
- No new inbound syslog listeners or additional servers.
- Supports recovery from locally persisted logs.
- Minimal disruption to the existing logging architecture.
- Lower implementation cost and complexity.
- Straightforward deployment and rollback.

**Cons**
- Additional file descriptor, CPU, memory, and disk I/O usage.
- Dependent on proper file rotation and offset management.
- Shares the Rsyslog server's infrastructure failure domain.
- Requires explicit Legal Entity and tenant mapping.
- Does not eliminate existing upstream UDP log loss risks.

### Option B – Fluent Bit Network Input on Existing Rsyslog Servers

**Pros**
- Reuses existing servers and client connectivity.
- Avoids Tail input and file monitoring overhead.
- Enables direct stream processing.
- Does not require additional standalone log collection servers.

**Cons**
- Existing Rsyslog listener ports cannot be directly reused by a separate Fluent Bit listener.
- Requires additional Rsyslog forwarding configuration.
- Introduces forwarding queue and backpressure management.
- Requires careful failure isolation to protect existing logging.
- Tenant identification and routing remain necessary.

### Option C – Dedicated Fluent Bit Servers

**Pros**
- Independent resources and failure isolation from existing Rsyslog servers.
- Greater flexibility for future scaling.
- Avoids Tail-related file monitoring overhead.
- Supports dedicated listener and Legal Entity routing design.
- Enables independent high availability architecture.

**Cons**
- Requires additional servers and maintenance.
- Requires Linux client-side configuration changes.
- Introduces additional network and firewall requirements.
- May require F5/load balancer configuration.
- Increases deployment and operational complexity.
- Higher infrastructure and implementation costs.

### Decision Driver Comparison

| Decision Driver | Option A | Option B | Option C |
|---|---|---|---|
| Infrastructure reuse | High | High | Low |
| Additional infrastructure cost | Low | Low | High |
| Linux client changes | None | None* | Required |
| Existing Rsyslog configuration changes | Minimal | Required | Minimal |
| Additional firewall impact | Low** | Low** | High |
| File monitoring overhead | Yes | No | No |
| Failure isolation from Rsyslog host | Low | Low | High |
| Downstream recovery | Local files + buffering | Forwarding queue + buffering | Requires independent buffering |
| Scalability | Moderate | Moderate | High |
| Operational complexity | Low–Medium | Medium | High |
| Implementation effort | Low | Medium | High |
| Reversibility before Splunk retirement | High | Medium–High | Medium |

*Option B assumes Rsyslog forwards logs to a separate local Fluent Bit listener.*

**Options A and B preserve existing inbound firewall rules but still require validation of outbound connectivity to LaaS.*

These are qualitative architectural assessments and do not represent measured performance benchmarks.

### Final Justification

Option A provides the best balance of operational simplicity, infrastructure reuse, cost efficiency, and manageable technical risk.

It enables Linux SREs to gain centralized system log visibility through LaaS while minimizing disruption to the existing Rsyslog environment.

The solution also supports the planned Splunk decommissioning and the firm's centralized logging and regulatory control objectives.

Therefore, **Option A is selected for implementation**, subject to completion of the required production validation and security controls.

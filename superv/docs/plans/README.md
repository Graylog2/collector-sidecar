# Implementation Plans

## Active Plans

| Plan | Status | Description |
|------|--------|-------------|
| [Phase 3](2026-01-29-opamp-supervisor-phase3.md) | TODO | Operational robustness (crash recovery, cert renewal) |
| [Phase 4](2026-01-29-opamp-supervisor-phase4.md) | TODO | Package management |

## Reference Documents

| Document | Description |
|----------|-------------|
| [Design](2026-01-23-opamp-supervisor-design.md) | Overall architecture and design decisions |

## Completed Plans

Executed plans are archived in [`completed/`](completed/):

| Plan | Description |
|------|-------------|
| [Phase 1](completed/2026-01-23-opamp-supervisor-implementation.md) | Project foundation |
| [Phase 2](completed/2026-01-23-opamp-supervisor-phase2.md) | Config management & health monitoring |
| [Template Expansion](completed/2026-01-28-template-expansion-design.md) | Agent args template expansion |
| [CSR Trust Bootstrap](completed/2026-01-29-csr-trust-bootstrap-design.md) | Authentication design |

## Execution Order

```
Phase 1 (Foundation)     ✅ DONE
    ↓
Phase 2 (Config/Health)  ✅ DONE
    ↓
Phase 3 (Robustness)     ⏳ NEXT
    ↓
Phase 4 (Packages)       📋 PLANNED
```

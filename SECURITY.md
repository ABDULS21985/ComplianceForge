# Security reporting policy

## Supported releases

Security fixes are produced for the currently deployed production release and
the latest release branch. Pre-release branches and unsupported historical
images may be investigated to determine exposure but do not receive separate
patches. Deployments must use immutable signed digests; mutable development
tags are not supported production artifacts.

## Report a vulnerability privately

Use this repository's **Security → Report a vulnerability** flow to open a
private security advisory. Do not open a public issue or include customer data,
credentials, access tokens, private evidence, or active exploit payloads in a
public channel. If private reporting is unavailable, contact the contractual
security contact for the deployment and reference this repository without
attaching sensitive data.

Include the affected version or image digest, component and route, prerequisites,
reproduction steps using synthetic data, impact, tenant-boundary implications,
and any suggested mitigation. State whether exploitation has been observed.

We target acknowledgement within one business day. Critical reports are
triaged immediately under the vulnerability-management runbook. Timelines vary
with reproducibility, affected customers, upstream dependencies, and safe
coordination; reporters receive status updates through the private advisory.

Do not test against customer or production systems without written
authorization, degrade service, persist access, access data beyond the minimum
synthetic proof, or socially engineer personnel. This policy does not grant
authorization beyond applicable law and the scope explicitly agreed by the
system owner.

## Disclosure and credit

ComplianceForge coordinates remediation, customer communication, upstream
notification, and CVE assignment where appropriate. Public disclosure timing
is agreed through the private advisory and must allow affected deployments a
reasonable remediation window. Reporter credit is offered with consent; a
reporter may remain anonymous.

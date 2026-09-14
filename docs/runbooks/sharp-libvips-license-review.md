# Sharp/libvips license review

Status: **blocked pending Security/Legal decision**  
Technical owner: Frontend Platform  
Decision owner: Security and Legal  
Review opened: 2026-09-14  
Exception expiry: not applicable until an exception is approved  
Next review due: 2026-09-28

## Finding and transitive chain

Trivy 0.74.0 reports 14 HIGH `restricted` license findings in the installed
frontend dependency graph. They are license findings, not known-vulnerability
findings:

```text
next@16.3.5
└── sharp@0.35.4
    ├── @img/sharp-libvips-{darwin-arm64,darwin-x64,linux-arm,
    │   linux-arm64,linux-ppc64,linux-riscv64,linux-s390x,linux-x64,
    │   linuxmusl-arm64,linuxmusl-x64} (LGPL-3.0-or-later)
    ├── @img/sharp-wasm32 (Apache-2.0 AND LGPL-3.0-or-later AND MIT)
    └── @img/sharp-win32-{arm64,ia32,x64}
        (Apache-2.0 AND LGPL-3.0-or-later)
```

These platform packages are optional native image-processing binaries selected
by npm for the deployment architecture. ComplianceForge does not directly
import `sharp`; Next.js uses it for server-side image optimization when the
runtime package is present. The production image and SBOM determine which
platform-specific binary is actually distributed, but the lockfile records all
supported variants and the source-license gate intentionally reports them all.

## Review considerations

No license exception is implied by optional or dynamic loading. Before shipping
an image containing libvips, counsel must confirm the distribution model and
the applicable LGPL-3.0-or-later duties. The review should at minimum cover:

- preservation of copyright and license notices;
- an accessible offer or location for the exact corresponding libvips source,
  including any modifications and build scripts required by the license;
- the user's ability to replace/relink the LGPL component where applicable;
- separation of proprietary application code from the LGPL library, and no
  static-linking or anti-replacement mechanism that changes the analysis;
- retention of the shipped image digest, npm lockfile, SBOM, notices, upstream
  source reference, and counsel's decision for the supported distribution term.

This checklist is an engineering evidence prompt, not legal advice.

## Decision options

1. **Governed exception.** Security/Legal approves distribution, records the
   precise packages/versions and image digests, required notices/source/relink
   controls, accountable owner, approval ticket, review cadence, and a finite
   expiry date. Only then may the license policy gain a narrowly scoped,
   version-bound exception.
2. **Remove the runtime.** Disable Next.js image optimization for deployed
   assets and remove the optional `sharp` runtime/binaries from the production
   dependency/image graph. Rebuild the frontend image, regenerate its SBOM, and
   prove the Trivy license gate has zero HIGH/CRITICAL results.
3. **Replace the image path.** Move optimization to an approved external image
   service or a dependency with an accepted license, then repeat build, SBOM,
   functional, performance, and license validation.

Until one option is approved and evidenced, the `LICENSE_TRIVY` CI result stays
blocking. Do not add a blanket license, package-pattern, severity, or scanner
suppression.

## Decision record (complete when approved)

- Decision: pending
- Ticket/risk record: pending
- Approved package versions and image digests: pending
- Required notices/source location: pending
- Compensating controls: pending
- Accountable implementation owner: Frontend Platform
- Security approver: pending
- Legal approver: pending
- Approval date: pending
- Exception expiry/review date: pending
- Revalidation evidence: pending

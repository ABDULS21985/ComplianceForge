# Accessibility and production-state audit

Audit window: 14–15 September 2026. Scope: the frontend source tree in this workspace, including all 62 `page.tsx` routes, the root/auth/authenticated shells, shared controls, route fallbacks, and representative browser workflows.

This is an engineering audit and evidence record, **not a WCAG 2.2 AA conformance claim or an accessibility certification**. Full-page and complete-process evaluation remains necessary under the [WCAG 2.2 conformance model](https://www.w3.org/TR/WCAG22/#conformance-reqs). Automated checks do not replace evaluation with disabled users and assistive technology.

## What was actually evaluated

- Static inspection of every routed page and its inherited shell/state path.
- Vitest/Testing Library interactions and `jest-axe` checks in jsdom for resource states, query recovery, connectivity, dialogs, and table controls.
- Deterministic style checks for representative text contrast pairs, reduced motion, focus outlines, coarse-pointer targets, forced colours, and overlay/table reflow structure.
- A dedicated Playwright/Chromium suite for the seven public account/token-portal entry routes: real-browser axe, 320-CSS-pixel horizontal-reflow checks, client route focus, keyboard traversal, reduced-motion transitions, and offline announcements. These runs cover initial account forms and missing-credential/unavailable-token outcomes, not successful invitation, authenticated board, or loaded vendor-assessment processes.
- Type generation, TypeScript, frontend lint, dependency audit, and production compilation.

No manual VoiceOver, NVDA, JAWS, TalkBack, switch-device, physical mobile-device, or disability-user sessions were performed. No manual browser-zoom, complete visual-contrast, or complete authenticated-process audit was performed. A 320-CSS-pixel viewport test is reflow evidence; it is not a claim that actual browser zoom was manually tested.

## Production changes and coverage model

The [root shell](../frontend/src/app/layout.tsx) now mounts a single route focus/title/announcement manager, connectivity banner, and active-query recovery status. Root loading, error, and not-found fallbacks also use the shared state system. Account pages, onboarding, both token portals, and authenticated session loading/failure states have a main landmark. Every non-redirect page source contains a primary `h1`.

The [query status safety net](../frontend/src/components/data/query-status.tsx) observes only currently mounted query consumers. It announces loading, distinguishes HTTP 403 from transport/service failure, identifies failed refetches with cached data as potentially stale, and retries active failed requests. It renders no raw server error/configuration details. Connectivity and failure banners are in normal document flow, rather than covering focused controls.

The six remaining direct-request resource routes—activity, knowledge, search, branding, board portal, and vendor portal—now provide explicit named loading states and safe error/retry states. Activity, knowledge, search, and branding distinguish permission denial; collection data is cleared on a denied direct read. Activity, knowledge, and search identify retained data after failed refreshes. Vendor assessment has an explicit no-question outcome and visible autosave failures.

Shared table search/filter controls are named, sort actions are native keyboard buttons with `aria-sort`, and clickable rows have an explicit keyboard action. Shared buttons/tabs can wrap text; tabs wrap within their container. Shared dialogs retain bounded viewport sizing and vertical scrolling. Global CSS provides visible focus, a 44px coarse-pointer control floor, forced-colour selection outlines, and reduced-motion fallbacks. The project uses a 44px target policy; WCAG AA's [target-size criterion](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html) has different minimum/spacing rules and exceptions.

Knowledge content no longer injects regex-generated HTML. Its deliberately small renderer emits safe text, headings, and lists, with one page-level heading and stable heading anchors. Rich inline Markdown features are not claimed as supported by this renderer.

Onboarding announces failed mutations, retains entered values, and advances only after a successful save/skip. Step changes focus the current step heading. Its step navigation and team/risk/control layouts now have narrow-screen structural fallbacks.

Coverage codes used below:

| Code | Evidence and limits |
| --- | --- |
| G | Root focus/title/announcement, offline warning, segment loading/error/not-found, shared controls. Applies to all rendered routes. |
| Q | React Query, directly or through domain hooks/panels; global named loading/failure/403/stale/retry safety net plus existing local collection/detail states. This is not proof that every legacy local state is fully standardized. |
| D | Explicit direct-request `ResourceState` loading/error/retry implementation. Singleton/portal empty states are contextual rather than inferred from response shape. |
| F | Form workflow: pending/disabled actions, validation/error announcements, and success/missing-credential outcomes. Collection empty/stale states are not applicable to a credential form. |
| R | Server redirect/quick-create alias; the destination owns the rendered state and focus semantics. |

## Complete routed-page inventory

There are 62 source routes: 57 rendered pages and five redirect aliases. This inventory is also guarded by [the source contract test](../frontend/src/lib/route-accessibility-contract.test.ts). Dynamic route parameters below are placeholders, not real record identifiers.

| Route | Shell/state path | Inspected empty/outcome scope |
| --- | --- | --- |
| `/` | R → dashboard | Redirect only |
| `/login` | G + F, auth main | Password/passkey/step-up/email-verification outcomes |
| `/forgot-password` | G + F, auth main | Account recovery confirmation |
| `/reset-password` | G + F, auth main | Missing credential and completion |
| `/verify-email` | G + F, auth main | Request/verification outcomes |
| `/accept-invitation` | G + F, auth main | Missing credential and acceptance |
| `/dashboard` | G + Q, domain boundaries | Summary, activity, incidents, trends |
| `/activity` | G + D | Filtered feed; keyboard load-more alternative |
| `/analytics` | G + Q, legacy local states | Trend/benchmark/incident collections |
| `/assets` | G + Q, typed domain hooks | Filtered inventory |
| `/assets/[id]` | G + Q, typed detail | Missing record, relationships, history |
| `/audits` | G + Q, domain boundaries | Filtered audits |
| `/audits/[id]` | G + Q, typed detail | Missing audit, findings, history |
| `/audits/new` | R → audit quick-create | Redirect only |
| `/bia` | G + Q, legacy local states | Processes, scenarios, plans, exercises, single points of failure |
| `/board` | G + Q, legacy local states | Meetings, reports, decisions, members |
| `/calendar` | G + Q, domain hook | Event list/calendar period |
| `/controls/[id]` | G + Q, typed detail/panels | Missing control, evidence/review/history |
| `/data` | G + Q, legacy local states | Processing-activity register |
| `/dsr` | G + Q, local states | Subject-request register/dashboard |
| `/dsr/[id]` | G + Q, local detail | Missing request, tasks/history |
| `/evidence` | G + Q, domain boundaries | Filtered evidence and expired/current version context |
| `/exceptions` | G + Q, legacy local states | Exception register/KPIs |
| `/frameworks` | G + Q, local states | Framework catalogue/adoption |
| `/frameworks/[id]` | G + Q, local states | Missing framework, controls, gaps, mappings |
| `/incidents` | G + Q, typed domain hooks | Filtered incidents/breach context |
| `/incidents/[id]` | G + Q, typed detail/panels | Missing incident, assignments/timeline/breach decisions |
| `/incidents/new` | R → incident quick-create | Redirect only |
| `/knowledge` | G + D | Filtered articles, unavailable content, optional recommendations |
| `/marketplace` | G + Q, legacy local states | Catalogue/installed/featured packages |
| `/monitoring` | G + Q, local states | Monitors, drift, automation configuration |
| `/nis2` | G + Q, local states | Assessment, measures, training, incidents |
| `/notifications` | G + Q, typed local states | Paginated notification feed/unread state |
| `/policies` | G + Q, domain boundaries | Filtered policies |
| `/policies/[id]` | G + Q, local states | Missing policy, content, versions, attestations, exceptions |
| `/policies/new` | R → policy quick-create | Redirect only |
| `/regulatory` | G + Q, local states | Regulatory changes/sources/dashboard |
| `/remediation` | G + Q, local states | Plans, gaps, selected plan/progress |
| `/reports` | G + Q, domain hooks | Report catalogue/generation/download |
| `/risks` | G + Q, domain boundaries | Filtered risks and heatmap |
| `/risks/[id]` | G + Q, local states | Missing risk, assessments, treatments, controls |
| `/risks/new` | R → risk quick-create | Redirect only |
| `/search` | G + D | Initial prompt, filtered results, suggestions, pagination |
| `/settings` | G + Q, domain boundaries | Organization/users/roles/activity |
| `/settings/access-policies` | G + Q, permission gates | Role catalogue and effective permissions |
| `/settings/access-policies/[id]` | G + Q, permission gates | Missing role, permission matrix, assignments/history |
| `/settings/branding` | G + D, singleton form | Load denied/unavailable; save/upload/domain outcomes |
| `/settings/capabilities` | G + Q, permission gates/panels | Capability catalogue, entitlements, override/history |
| `/settings/data-governance` | G + Q, permission/capability gates/panels | Policy, schedules, records, exceptions, holds, verification/history |
| `/settings/diagnostics` | G + Q, permission gates | Safe health/posture snapshot, stale/freshness/retry |
| `/settings/identity` | G + Q + F, permission gates | Identity policy, invitation/admin reset/history |
| `/settings/integrations` | G + Q, permission gates/panels | Catalogue, configured connectors, logs, SSO, API keys |
| `/settings/notifications` | G + Q, permission gates/panels | Templates, rules, channels, safe tests |
| `/settings/security` | G + Q + F, permission gates/panels | Sessions, MFA/recovery codes, passkeys/step-up |
| `/settings/subscription` | G + Q, local states | Current subscription/plans/usage |
| `/vendor-assessments` | G + Q, legacy local states | Assessments, questionnaires, statistics |
| `/vendors` | G + Q, domain boundaries | Filtered vendor register |
| `/vendors/[id]` | G + Q, local states | Missing vendor, assessments, contacts/documents |
| `/workflows` | G + Q, local states | Approval/workflow register |
| `/onboard` | G + Q + F, own main | Step form, save/skip/launch outcomes |
| `/board-portal` | G + D, own main | Invalid/missing invitation and board snapshot |
| `/vendor-portal` | G + D + F, own main | Invalid/missing invitation, no questions, autosave/submission |

## WCAG evidence mapping

Criterion identifiers below refer to the [official WCAG 2.2 Recommendation](https://www.w3.org/TR/WCAG22/). “Evidence” is limited to the listed implementation/tests; it is not an all-pages pass verdict.

| Area | Relevant criteria | Evidence | Remaining evaluation |
| --- | --- | --- | --- |
| Landmarks, headings, tables, labels | 1.3.1, 2.4.1, 2.4.6, 4.1.2 | Route inventory; auth/session landmarks; table names/scopes/sort state; named filters; public browser axe | Full authenticated page/third-party content inspection |
| Colour and contrast | 1.4.1, 1.4.3, 1.4.11 | Text/icon state cues; five deterministic normal-text palette pairs; real-browser public-shell axe | All loaded domain charts, dark/custom themes, hover/disabled/focus states |
| Text resize/reflow/spacing | 1.4.4, 1.4.10, 1.4.12 | 320-CSS-pixel public-shell checks; wrapping buttons/tabs; responsive onboarding; bounded dialog/table structure | Actual 200%/400% browser zoom and text-spacing overrides across authenticated processes |
| Keyboard/focus | 2.1.1, 2.1.2, 2.4.3, 2.4.7, 2.4.11 | Native table sort/row actions; Escape/trigger restoration; route-focus tests; visible focus CSS; normal-flow banners | Keyboard-only completion of every domain mutation and assistive-technology focus validation; [focus obscuration](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html) with all sticky controls/toasts |
| Target size/drag alternatives | 2.5.7, 2.5.8 | Shared 44px controls/dialog close; coarse-pointer floor; branding upload button; activity load-more button | Physical touch-device measurements and all bespoke inline-control exceptions |
| Errors, authentication, status | 3.3.1–3.3.4, 3.3.8, 4.1.3 | Safe retry/403/stale/offline statuses; form errors; one-time credential outcomes; save-before-advance onboarding | Full account recovery/MFA/passkey process with password managers and screen readers |
| Motion | 2.2.2; additional reduced-motion support | Global motion media query; real-browser reduced-motion transition check | All custom chart/third-party animation behavior |

## Exceptions and acceptance blockers

These are unresolved evaluation/implementation items, not approved waivers from WCAG.

| ID | Gap / exact boundary | Required acceptance evidence |
| --- | --- | --- |
| A11Y-01 | No manual screen-reader or disability-user audit was performed. | Documented VoiceOver/Safari and NVDA/Chrome or Firefox runs, plus agreed mobile assistive-technology coverage, for complete priority processes. |
| A11Y-02 | Authenticated routes received source/shared-component coverage, not a real-browser axe run for every loaded tab, error, empty, mutation, and permission combination. | Stable tenant fixtures and a permission/capability matrix; full routed browser/axe states with retained artifacts. |
| A11Y-03 | Legacy analytics/BIA/board/marketplace and other bespoke metadata still use low-contrast `text-gray-400` styles. The tested palette is not a global contrast pass. | Replace meaningful low-contrast text; measure all light/dark/custom-theme foreground/background and non-text-control pairs. |
| A11Y-04 | Recharts and bespoke analytics visuals do not all have complete keyboard-accessible data equivalents. Pointer tooltips alone are insufficient evidence. | Accessible table/summary equivalents for every meaningful chart and keyboard tooltip/interaction tests. |
| A11Y-05 | Legacy local empty/mutation states remain heterogeneous. Root query recovery supplies loading/error/403/stale semantics but cannot infer domain-empty response shapes or rewrite every local mutation. | Migrate legacy analytics, BIA, board, data, exceptions, marketplace, monitoring, NIS2, remediation, vendor-assessment, and workflow states to explicit domain boundaries; test every mutation outcome. |
| A11Y-06 | Global query denial is an honest recovery notice, not a replacement for page-specific permission boundaries. Some legacy cached-query views do not yet hide cached content when permissions are revoked mid-session. | Domain permission boundaries and permission-change cache invalidation tests; never present retained protected content as current authorized data. |
| A11Y-07 | Actual browser zoom, text-spacing overrides, high-contrast OS modes, physical mobile devices, and landscape were not manually tested. | Viewport/zoom/text-spacing/device matrix with screenshots and keyboard checks, including table/dialog/sticky-control behavior. |
| A11Y-08 | User-authored policy HTML, uploaded documents, knowledge content, translations, and vendor-supplied content are not certified by shared-shell tests. | Authoring constraints, accessible-content guidance, content QA, and independently evaluated representative documents. |
| A11Y-09 | Onboarding still has legacy fallback recommendation/control content when the service does not return those fields; the global error is explicit, but recommendations are not a validated offline dataset. | Replace fallback product data with typed service outcomes and test accessible unavailable/empty recommendation states. |

## Reproducible commands and final gate record

Run from `frontend/`. The dedicated browser suite needs a completed production build and no backend fixture for its public-shell checks.

```sh
npm run type-check
npm test -- --maxWorkers=1 --no-file-parallelism
npx eslint . --max-warnings=315
npm audit --audit-level=low
npm run build
npx playwright test --config=playwright.accessibility.config.ts
```

Install the matching browser once with `npx playwright install chromium` if the local Playwright cache lacks it. The dedicated server script uses the production standalone output and copies its static assets into that generated output; it does not run a mock frontend or change the BFF.

Final gate results are recorded below and in the W36 handoff. Tests must not be run concurrently in a resource-constrained process and then treated as an accessibility verdict; transient timeout failures require an isolated rerun and disclosure.

| Gate | Result |
| --- | --- |
| `npm run type-check` (`next typegen && tsc --noEmit`) | Passed |
| Full Vitest, isolated with one worker and no file parallelism | Passed, 77/77 files and 326/326 tests, 148.86s |
| Full ESLint with the existing rules, `--max-warnings=315` | Passed, 328 files, 0 errors, 315 warnings (20 fewer than the W35 checkpoint) |
| W36 touched TypeScript/config/browser-script ESLint, `--max-warnings=0` | Passed, 35 files, 0 warnings |
| `npm audit --audit-level=low` | Passed, 0 vulnerabilities |
| `npm run build` | Passed, Next 16.3.5 production build, 54/54 generated pages |
| Dedicated production-browser accessibility suite | Passed, 9/9 Chromium checks, 36.2s |

The local gates ran with Node 25.2.1; the dedicated browser was Chromium 153.0.8010.12, installed through Playwright's matching cache. The static-page build count is Next.js's build summary, not the source-route inventory count.

The first browser attempt required installation of the matching Chromium cache. A subsequent portal test timed out on `networkidle` even though its accessible error outcome was rendered; the readiness condition was corrected to await that named outcome. No axe rule, target-size assertion, focus assertion, or timeout threshold was weakened. Concurrent unit/browser/Go activity, and a later multi-project-pool attempt under host load, caused existing 5-second dialog tests to time out; those contended runs were stopped and are not the final unit gate. The final fully serialized suite passed without changing thresholds. All owned test/browser/standalone-server processes were stopped after completion.

## Roadmap disposition

- Item 34 remains **Partial**: shared production coverage is route-wide, but A11Y-05/06/09 prevent a claim that every domain-specific state is standardized.
- Item 36 remains **Partial**: automated evidence and documented exceptions exist; A11Y-01/02/03/04/07/08 block WCAG 2.2 AA conformance.
- Item 37 is **Verified** under the delivery ledger's production-path/proportionate-test definition: its shared implementation is covered by route focus/announcements, accessible dialog restoration, touch-target/motion policies, keyboard table affordances, and non-colour state cues. Full conformance/device assurance remains governed by item 36; no manual-test claim is made here.

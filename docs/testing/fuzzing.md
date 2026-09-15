# Bounded fuzz testing

`make test-fuzz-smoke` runs the security and state-machine fuzz targets for a
bounded period. CI sets `FUZZ_TIME=2s` per target; local investigations can use
a longer value, for example `FUZZ_TIME=30s make test-fuzz-smoke`.

The campaign covers:

- versioned queue envelope decoding, validation, and stable round trips;
- outbound webhook URL policy and private/local destination rejection;
- local object-storage path containment;
- attachment filename/header safety and CSS active-content sentinels;
- directory CSV and dynamic group-rule parsers;
- finding and incident state transitions;
- pagination and audit-date normalization; and
- protected-route permission mapping.

Fuzz functions retain small, security-relevant seed corpora and cap individual
input sizes. A discovered crashing input is a release blocker. Commit the
minimized corpus entry under the package's `testdata/fuzz/<target>` directory
before fixing the defect so the regression remains deterministic in ordinary
`go test` runs.

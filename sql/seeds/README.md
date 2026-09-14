# Database seeds

`cmd/seed` is the supported reference-data bootstrap. It reads
`manifest.txt`, applies each file in the declared order, and records its SHA-256
checksum and position in `bootstrap_seed_history`.

```bash
DATABASE_URL='postgres://user:password@localhost:5432/complianceforge?sslmode=disable' \
  go run ./cmd/migrate up
DATABASE_URL='postgres://user:password@localhost:5432/complianceforge?sslmode=disable' \
  go run ./cmd/seed
```

Re-running the seed command is safe. An applied seed is immutable: changing its
contents or position fails closed instead of partially overwriting reference
data. Add new production reference data in a new file appended to the manifest.
Each seed file must retain its `BEGIN;` / `COMMIT;` wrapper; the runner removes
that wrapper and executes the contents with the history insert in one database
transaction.

Do not restore the former alphabetical `psql sql/seeds/*.sql` loop. Filename
order does not express dependencies, and several files are intentionally not
global bootstrap data.

## Packs outside the supported manifest

These files remain available for future repair or tenant-provisioning work, but
`cmd/seed` deliberately does not execute them:

| File | Reason excluded |
| --- | --- |
| `access_policies.sql` | Tenant-specific examples use a zero UUID placeholder that is not a real organization. |
| `data_categories.sql` | Tenant-specific templates insert NULL organization IDs into required tenant columns and use stale category values. |
| `evidence_templates.sql` | Legacy template IDs use the non-UUID `et...` prefix. |
| `marketplace_packages.sql` | Legacy publisher/package IDs and columns no longer match migration 026. |
| `questionnaire_templates.sql` | Targets the removed `questionnaire_templates` model rather than migration 032's assessment questionnaire model. |
| `workflow_definitions.sql` | Legacy workflow IDs use the non-UUID `g...` prefix. |

Tenant-specific defaults should be installed by an organization-provisioning
workflow with a real tenant ID, not by the global database bootstrap.

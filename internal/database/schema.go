package database

// SupportedSchemaVersion is the newest database migration understood by this
// application build. Migration validation keeps this value synchronized with
// the highest migration file so diagnostics can detect application/database
// drift without reading the source tree at runtime.
const SupportedSchemaVersion int64 = 59

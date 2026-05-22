package plugin

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// pluginTablePrefix is the mandatory prefix for all plugin-created SQL tables.
// Each plugin's tables must start with "plugin_<name>_".
const pluginTablePrefix = "plugin_"

// coreAllowedTable is the table name the migration system itself needs to access.
const coreAllowedTable = "plugin_migrations"

// healthCheckQuery is the only allowed table-less query (for ping / liveness).
const healthCheckQuery = "SELECT 1"

// SQLTableGate enforces that a plugin can only access tables it has registered.
// Tables are automatically registered during migration and must follow the
// naming convention "plugin_<name>_*". Thread-safe.
type SQLTableGate struct {
	mu     sync.RWMutex
	tables map[string]map[string]bool // pluginName -> set of allowed table names
}

// NewSQLTableGate returns an empty gate. Use AllowTable to register tables.
func NewSQLTableGate() *SQLTableGate {
	return &SQLTableGate{tables: make(map[string]map[string]bool)}
}

// AllowTable registers a table name for the given plugin.
// The table must start with "plugin_<pluginName>_" or be the
// core-allowed migration table.
func (g *SQLTableGate) AllowTable(pluginName, tableName string) error {
	if err := validateTableOwnership(pluginName, tableName); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.tables[pluginName] == nil {
		g.tables[pluginName] = make(map[string]bool)
	}
	g.tables[pluginName][tableName] = true
	return nil
}

// allowedStatementPrefixes whitelists the SQL command verbs that plugins may issue.
// Anything not starting with one of these is rejected outright. Note: this is a
// strict prefix list (the leading word), evaluated AFTER the leading-verb rejection
// in dangerousLeadingVerbs — so e.g. "SET" never reaches this check.
var allowedStatementPrefixes = []string{"SELECT", "INSERT", "UPDATE", "DELETE", "WITH"}

// dangerousLeadingVerbs are top-level statement verbs that plugins must never
// issue. They are rejected when they appear as the FIRST token of the query.
// Keeping this list separate from dangerousSQLPattern is critical: words like
// SET also appear inside legitimate UPDATE statements ("UPDATE t SET c = 1"),
// and DO appears inside "INSERT ... ON CONFLICT DO NOTHING". A naive substring
// check would block both.
var dangerousLeadingVerbs = []string{
	"SET", "SHOW", "RESET", "DISCARD",
	"PREPARE", "EXECUTE", "DEALLOCATE", "DO", "CALL",
	"EXPLAIN", "ANALYZE",
	"DROP", "ALTER", "TRUNCATE", "GRANT", "REVOKE",
	"VACUUM", "REINDEX", "CLUSTER", "LOCK",
	"LISTEN", "NOTIFY", "COPY", "LOAD",
	"COMMENT",
	"CREATE",
}

// dangerousSQLPattern matches SQL fragments that plugins must never use,
// regardless of position in the statement. Unlike dangerousLeadingVerbs, this
// targets identifiers whose presence ANYWHERE indicates an escape attempt
// (PG superuser-only functions, file-system helpers, server-side raise).
//
// Note: prefix patterns like "pg_read_" intentionally omit the trailing \b
// because '_' is a \w character — \b would not match between '_' and a
// following word char like 'server'. Anchoring only at the left handles
// pg_read_server_files / pg_read_binary_file etc.
var dangerousSQLPattern = regexp.MustCompile(
	`(?i)(?:` +
		`\bpg_read_|\bpg_ls_|\bpg_stat_file\b|\bpg_sleep\b|` +
		`\blo_import\b|\blo_export\b|` +
		`\bRAISE\s+(?:NOTICE|WARNING|EXCEPTION|INFO|DEBUG|LOG)\b` +
		`)`,
)

// schemaQualifiedAfterClausePattern flags schema.table appearing right after
// a clause that introduces a table reference (FROM, INTO, UPDATE, JOIN,
// DELETE FROM). We don't ban ALL dotted identifiers because legitimate
// "table_alias.column" references inside SELECT lists / WHERE clauses are
// common and harmless. Reaching into another schema, however, requires
// "schema.table" right after one of the table-introducing clauses.
var schemaQualifiedAfterClausePattern = regexp.MustCompile(
	`(?i)\b(?:FROM|INTO|UPDATE|JOIN|DELETE\s+FROM)\s+"?\w+"?\s*\.\s*"?\w+"?`,
)

// stringLiteralPattern strips PostgreSQL single-quoted string literals so they
// can't smuggle a "schema.table" sequence past schemaQualifiedAfterClausePattern.
// Doubled '' inside literals is handled by the alternation [^'] | ''.
var stringLiteralPattern = regexp.MustCompile(`'(?:[^']|'')*'`)

// CheckQuery validates a plugin-issued SQL string against the gate's policy:
//  1. leading verb is not in dangerousLeadingVerbs (SET / SHOW / DROP / etc.)
//  2. starts with a whitelisted DML verb (SELECT/INSERT/UPDATE/DELETE/WITH)
//  3. only one statement (no embedded ';')
//  4. contains no always-forbidden fragments (pg_sleep, pg_read_*, RAISE NOTICE, ...)
//  5. no schema-qualified identifiers ("schema.table")
//  6. every referenced table belongs to this plugin (or is the migrations table)
//
// Order matters: verb check runs first so payloads like `DO $$ ...; ... $$`
// are reported as "DO is forbidden" rather than "multi-statement".
// The literal `SELECT 1` is allowed as a health-check escape hatch.
func (g *SQLTableGate) CheckQuery(pluginName, query string) error {
	trimmed := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(query), ";"))
	if strings.EqualFold(trimmed, healthCheckQuery) {
		return nil
	}
	leading := leadingVerb(trimmed)
	if isDangerousLeadingVerb(leading) {
		return fmt.Errorf("plugin %q denied: statement verb %q is forbidden", pluginName, leading)
	}
	if !hasAllowedPrefix(leading) {
		return fmt.Errorf("plugin %q denied: statement must start with SELECT/INSERT/UPDATE/DELETE/WITH", pluginName)
	}
	if strings.Contains(trimmed, ";") {
		return fmt.Errorf("plugin %q denied: multi-statement SQL is not allowed", pluginName)
	}
	if dangerousSQLPattern.MatchString(query) {
		return fmt.Errorf("plugin %q denied: SQL contains forbidden keyword", pluginName)
	}
	if hasSchemaQualifiedIdentifier(query) {
		return fmt.Errorf("plugin %q denied: schema-qualified identifiers are not allowed", pluginName)
	}
	tables := extractTableNames(query)
	if len(tables) == 0 {
		return fmt.Errorf("plugin %q denied: query must reference at least one plugin table", pluginName)
	}
	return g.checkTablesAllowed(pluginName, tables)
}

// leadingVerbPattern captures the first identifier word at the start of the query.
var leadingVerbPattern = regexp.MustCompile(`^\s*(\w+)`)

// leadingVerb returns the first SQL word in upper case, or "" when the query
// is empty/non-alphabetic.
func leadingVerb(query string) string {
	m := leadingVerbPattern.FindStringSubmatch(query)
	if len(m) < 2 {
		return ""
	}
	return strings.ToUpper(m[1])
}

// isDangerousLeadingVerb reports whether verb is one of the always-forbidden
// statement-leading commands.
func isDangerousLeadingVerb(verb string) bool {
	for _, v := range dangerousLeadingVerbs {
		if verb == v {
			return true
		}
	}
	return false
}

// hasAllowedPrefix returns true when verb is one of the whitelisted DML verbs.
func hasAllowedPrefix(verb string) bool {
	for _, prefix := range allowedStatementPrefixes {
		if verb == prefix {
			return true
		}
	}
	return false
}

// hasSchemaQualifiedIdentifier returns true when the query references a
// schema-qualified table after a table-introducing clause (FROM/JOIN/etc.),
// excluding matches that occur inside quoted string literals.
func hasSchemaQualifiedIdentifier(query string) bool {
	stripped := stringLiteralPattern.ReplaceAllString(query, "''")
	return schemaQualifiedAfterClausePattern.MatchString(stripped)
}

// checkTablesAllowed verifies every referenced table belongs to this plugin
// (or is the shared coreAllowedTable). Reads the registry under RLock.
func (g *SQLTableGate) checkTablesAllowed(pluginName string, tables []string) error {
	g.mu.RLock()
	allowed := g.tables[pluginName]
	g.mu.RUnlock()
	for _, t := range tables {
		if t == coreAllowedTable {
			continue
		}
		if !allowed[t] {
			return fmt.Errorf("plugin %q denied access to table %q", pluginName, t)
		}
	}
	return nil
}

// validateTableOwnership checks that a table name follows the plugin prefix convention.
func validateTableOwnership(pluginName, tableName string) error {
	if tableName == coreAllowedTable {
		return nil
	}
	expectedPrefix := pluginTablePrefix + pluginName + "_"
	if !strings.HasPrefix(tableName, expectedPrefix) {
		return fmt.Errorf("table %q must start with %q", tableName, expectedPrefix)
	}
	return nil
}

// tableNamePattern matches table identifiers after SQL keywords.
// Captures optional schema-qualified names: "schema.table" or just "table".
// Handles optional double-quoting of identifiers. Schema-qualified matches
// are rejected upstream by hasSchemaQualifiedIdentifier; this regex still
// captures the schema slot so resolveTableName can flag the violation.
var tableNamePattern = regexp.MustCompile(
	`(?i)\b(?:FROM|INTO|UPDATE|JOIN|DELETE\s+FROM)\s+` +
		`"?(\w+)"?(?:\."?(\w+)"?)?`,
)

// extractTableNames returns all distinct table names referenced in a SQL query.
// It scans for FROM, INTO, UPDATE, JOIN, DELETE FROM clauses. Schema-qualified
// references resolve to "" so callers can distinguish "no tables" from
// "rejected name"; this code path is only reached after
// hasSchemaQualifiedIdentifier has already rejected schema qualification, so
// "" can be safely treated as an empty match here.
func extractTableNames(query string) []string {
	matches := tableNamePattern.FindAllStringSubmatch(query, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := resolveTableName(m)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// resolveTableName picks the table part from a regex match.
// m[1] is the unqualified identifier; m[2] is the schema slot for "schema.table".
// Schema-qualified matches return "" — callers must reject schema qualification
// upstream (see hasSchemaQualifiedIdentifier). Returning "" here ensures that even
// if hasSchemaQualifiedIdentifier is bypassed, the table never falls through to
// the allow-list as if it were unqualified.
func resolveTableName(m []string) string {
	if len(m) >= 3 && m[2] != "" {
		return ""
	}
	if len(m) >= 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

// createTablePattern matches CREATE TABLE statements to extract the table name.
var createTablePattern = regexp.MustCompile(
	`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"?(\w+)"?(?:\."?(\w+)"?)?`,
)

// resolveCreatedTableName mirrors resolveTableName but is used by
// RegisterTablesFromSQL where schema-qualified names are simply skipped (we only
// register unqualified names; schema-qualified DDL is rejected when applied).
func resolveCreatedTableName(m []string) string {
	if len(m) >= 3 && m[2] != "" {
		return ""
	}
	if len(m) >= 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

// RegisterTablesFromSQL scans SQL content for CREATE TABLE statements
// and registers each found table for the given plugin.
func (g *SQLTableGate) RegisterTablesFromSQL(pluginName, sqlContent string) error {
	matches := createTablePattern.FindAllStringSubmatch(sqlContent, -1)
	for _, m := range matches {
		name := resolveCreatedTableName(m)
		if name == "" {
			continue
		}
		if err := g.AllowTable(pluginName, name); err != nil {
			return err
		}
	}
	return nil
}

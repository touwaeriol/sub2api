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

// CheckQuery parses SQL table references and rejects any that are
// not in the plugin's allowed set. Returns nil if all tables are allowed.
func (g *SQLTableGate) CheckQuery(pluginName, query string) error {
	tables := extractTableNames(query)
	if len(tables) == 0 {
		return nil
	}
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
// Handles optional double-quoting of identifiers.
var tableNamePattern = regexp.MustCompile(
	`(?i)\b(?:FROM|INTO|UPDATE|JOIN|DELETE\s+FROM)\s+` +
		`"?(\w+)"?(?:\."?(\w+)"?)?`,
)

// extractTableNames returns all distinct table names referenced in a SQL query.
// It scans for FROM, INTO, UPDATE, JOIN, DELETE FROM clauses.
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
// m[1] is always present; m[2] is set for schema-qualified "schema.table".
func resolveTableName(m []string) string {
	if len(m) >= 3 && m[2] != "" {
		return strings.ToLower(m[2])
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

// RegisterTablesFromSQL scans SQL content for CREATE TABLE statements
// and registers each found table for the given plugin.
func (g *SQLTableGate) RegisterTablesFromSQL(pluginName, sqlContent string) error {
	matches := createTablePattern.FindAllStringSubmatch(sqlContent, -1)
	for _, m := range matches {
		name := resolveTableName(m)
		if name == "" {
			continue
		}
		if err := g.AllowTable(pluginName, name); err != nil {
			return err
		}
	}
	return nil
}
